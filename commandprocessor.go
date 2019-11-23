package gcn3

import (
	"log"
	"math"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/pagemigrationcontroller"
	"gitlab.com/akita/gcn3/rdma"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/idealmemcontroller"
	"gitlab.com/akita/mem/vm/addresstranslator"
	"gitlab.com/akita/mem/vm/tlb"
	"gitlab.com/akita/util"
	"gitlab.com/akita/util/akitaext"
	"gitlab.com/akita/util/tracing"
)

// CommandProcessor is an Akita component that is responsible for receiving
// requests from the driver and dispatch the requests to other parts of the
// GPU.
//
//     ToDriver <=> Receive request and send feedback to the driver
//     ToDispatcher <=> Dispatcher of compute kernels
type CommandProcessor struct {
	*akita.TickingComponent

	Dispatcher         akita.Port
	DMAEngine          akita.Port
	Driver             akita.Port
	TLBs               []akita.Port
	CUs                []akita.Port
	AddressTranslators []akita.Port
	RDMA               akita.Port
	PMC                akita.Port
	L1VCaches          []akita.Port
	L1SCaches          []akita.Port
	L1ICaches          []akita.Port
	L2Caches           []akita.Port
	DRAMControllers    []*idealmemcontroller.Comp

	ToDriver                   akita.Port
	toDriverSender             akitaext.BufferedSender
	ToDispatcher               akita.Port
	toDispatcherSender         akitaext.BufferedSender
	ToCUs                      akita.Port
	toCUsSender                akitaext.BufferedSender
	ToTLBs                     akita.Port
	toTLBsSender               akitaext.BufferedSender
	ToAddressTranslators       akita.Port
	toAddressTranslatorsSender akitaext.BufferedSender
	ToCaches                   akita.Port
	toCachesSender             akitaext.BufferedSender
	ToRDMA                     akita.Port
	toRDMASender               akitaext.BufferedSender
	ToPMC                      akita.Port
	toPMCSender                akitaext.BufferedSender

	kernelFixedOverheadInCycles int

	currShootdownRequest *ShootDownCommand
	currFlushRequest     *FlushCommand

	numCUs                uint64
	numTLBs               uint64
	numCUAck              uint64
	numAddrTranslationAck uint64
	numTLBAck             uint64
	numCacheACK           uint64

	shootDownInProcess bool

	bottomReqIDToTopReqMap map[string]*LaunchKernelReq
}

//nolint:gocyclo,funlen
// Handle processes the events that is scheduled for the CommandProcessor
func (p *CommandProcessor) Handle(e akita.Event) error {
	p.Lock()
	defer p.Unlock()

	switch evt := e.(type) {
	case akita.TickEvent:
		p.TickingComponent.Handle(e)
	case *ReplyKernelCompletionEvent:
		p.handleReplyKernelCompletionEvent(evt)
	default:
		log.Panicf("cannot handle event %s", reflect.TypeOf(evt))
	}
	return nil
}

func (p *CommandProcessor) Tick(now akita.VTimeInSec) bool {
	p.Lock()
	defer p.Unlock()

	madeProgress := false

	madeProgress = p.sendMsgsOut(now) || madeProgress
	madeProgress = p.processReqFromDriver(now) || madeProgress
	madeProgress = p.processRspFromInternal(now) || madeProgress

	return madeProgress
}

func (p *CommandProcessor) sendMsgsOut(now akita.VTimeInSec) bool {
	madeProgress := false

	madeProgress = p.toDriverSender.Tick(now) || madeProgress
	madeProgress = p.toDispatcherSender.Tick(now) || madeProgress
	madeProgress = p.toCUsSender.Tick(now) || madeProgress
	madeProgress = p.toTLBsSender.Tick(now) || madeProgress
	madeProgress = p.toAddressTranslatorsSender.Tick(now) || madeProgress
	madeProgress = p.toCachesSender.Tick(now) || madeProgress
	madeProgress = p.toRDMASender.Tick(now) || madeProgress
	madeProgress = p.toPMCSender.Tick(now) || madeProgress

	return madeProgress
}

func (p *CommandProcessor) processReqFromDriver(now akita.VTimeInSec) bool {
	msg := p.ToDriver.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *LaunchKernelReq:
		return p.processLaunchKernelReq(now, req)
	case *FlushCommand:
		return p.processFlushCommand(now, req)
	case *MemCopyD2HReq, *MemCopyH2DReq:
		return p.processMemCopyReq(now, req)
	case *RDMADrainCmdFromDriver:
		return p.processRDMADrainCmd(now, req)
	case *RDMARestartCmdFromDriver:
		return p.processRDMARestartCommand(now, req)
	case *ShootDownCommand:
		return p.processShootdownCommand(now, req)
	case *GPURestartReq:
		return p.processGPURestartReq(now, req)
	case *PageMigrationReqToCP:
		return p.processPageMigrationReq(now, req)
	}

	panic("never")
}

func (p *CommandProcessor) processRspFromInternal(now akita.VTimeInSec) bool {
	madeProgress := false

	madeProgress = p.processRspFromACEs(now) || madeProgress
	madeProgress = p.processRspFromRDMAs(now) || madeProgress
	madeProgress = p.processRspFromCUs(now) || madeProgress
	madeProgress = p.processRspFromATs(now) || madeProgress
	madeProgress = p.processRspFromCaches(now) || madeProgress
	madeProgress = p.processRspFromTLBs(now) || madeProgress
	madeProgress = p.processRspFromPMC(now) || madeProgress

	return madeProgress
}

func (p *CommandProcessor) processRspFromACEs(now akita.VTimeInSec) bool {
	msg := p.ToDispatcher.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *LaunchKernelReq:
		return p.processLaunchKernelRsp(now, req)
	case *MemCopyD2HReq, *MemCopyH2DReq:
		return p.processMemCopyRsp(now, req)
	}

	panic("never")
}

func (p *CommandProcessor) processRspFromRDMAs(now akita.VTimeInSec) bool {
	msg := p.ToRDMA.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *rdma.RDMADrainRsp:
		return p.processRDMADrainRsp(now, req)
	}

	panic("never")
}

func (p *CommandProcessor) processRspFromCUs(now akita.VTimeInSec) bool {
	msg := p.ToCUs.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *CUPipelineFlushRsp:
		return p.processCUPipelineFlushRsp(now, req)
	case *CUPipelineRestartRsp:
		return p.processCUPipelineRestartRsp(now, req)
	}

	panic("never")
}

func (p *CommandProcessor) processRspFromCaches(now akita.VTimeInSec) bool {
	msg := p.ToCaches.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *cache.FlushRsp:
		return p.processCacheFlushRsp(now, req)
	case *cache.RestartRsp:
		return p.processCacheRestartRsp(now, req)
	}

	panic("never")
}

func (p *CommandProcessor) processRspFromATs(now akita.VTimeInSec) bool {
	msg := p.ToAddressTranslators.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *addresstranslator.AddressTranslatorFlushRsp:
		return p.processAddressTranslatorFlushRsp(now, req)
	case *addresstranslator.AddressTranslatorRestartRsp:
		return p.processAddressTranslatorRestartRsp(now, req)
	}

	panic("never")
}

func (p *CommandProcessor) processRspFromTLBs(now akita.VTimeInSec) bool {
	msg := p.ToTLBs.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *tlb.TLBFlushRsp:
		return p.processTLBFlushRsp(now, req)
	case *tlb.TLBRestartRsp:
		return p.processTLBRestartRsp(now, req)
	}

	panic("never")
}

func (p *CommandProcessor) processRspFromPMC(now akita.VTimeInSec) bool {
	msg := p.ToPMC.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *pagemigrationcontroller.PageMigrationRspFromPMC:
		return p.processPageMigrationRsp(now, req)
	}

	panic("never")
}

func (p *CommandProcessor) processLaunchKernelReq(
	now akita.VTimeInSec,
	req *LaunchKernelReq,
) bool {
	reqToBottom := NewLaunchKernelReq(now, p.ToDispatcher, p.Dispatcher)
	reqToBottom.PID = req.PID
	reqToBottom.Packet = req.Packet
	reqToBottom.PacketAddress = req.PacketAddress
	reqToBottom.HsaCo = req.HsaCo
	reqToBottom.SendTime = now
	reqToBottom.WGFilter = req.WGFilter

	p.toDispatcherSender.Send(reqToBottom)
	p.bottomReqIDToTopReqMap[reqToBottom.ID] = req
	p.ToDriver.Retrieve(now)

	tracing.TraceReqReceive(req, now, p)
	tracing.TraceReqInitiate(reqToBottom, now, p,
		tracing.MsgIDAtReceiver(req, p))

	return true
}

func (p *CommandProcessor) processLaunchKernelRsp(
	now akita.VTimeInSec,
	rsp *LaunchKernelReq,
) bool {
	evt := NewReplyKernelCompletionEvent(
		p.Freq.NCyclesLater(p.kernelFixedOverheadInCycles, now),
		p, rsp,
	)
	p.Engine.Schedule(evt)
	p.ToDispatcher.Retrieve(now)
	return true
}

func (p *CommandProcessor) handleReplyKernelCompletionEvent(
	evt *ReplyKernelCompletionEvent,
) {
	now := evt.Time()

	req := evt.Req
	originalReq := p.bottomReqIDToTopReqMap[req.ID]
	originalReq.SendTime = now
	originalReq.Src, originalReq.Dst = originalReq.Dst, originalReq.Src

	p.toDriverSender.Send(originalReq)

	tracing.TraceReqFinalize(req, now, p)
	tracing.TraceReqComplete(originalReq, now, p)

	p.TickLater(now)
}

func (p *CommandProcessor) processRDMADrainCmd(
	now akita.VTimeInSec,
	cmd *RDMADrainCmdFromDriver,
) bool {
	req := rdma.RDMADrainReqBuilder{}.
		WithSendTime(now).
		WithSrc(p.ToRDMA).
		WithDst(p.RDMA).
		Build()

	p.toRDMASender.Send(req)
	p.ToDriver.Retrieve(now)

	return true
}

func (p *CommandProcessor) processRDMADrainRsp(
	now akita.VTimeInSec,
	rsp *rdma.RDMADrainRsp,
) bool {
	req := NewRDMADrainRspToDriver(now, p.ToDriver, p.Driver)

	p.toDriverSender.Send(req)
	p.ToRDMA.Retrieve(now)

	return true
}

func (p *CommandProcessor) processShootdownCommand(
	now akita.VTimeInSec,
	cmd *ShootDownCommand,
) bool {
	if p.shootDownInProcess == true {
		return false
	}

	p.currShootdownRequest = cmd
	p.shootDownInProcess = true

	for i := 0; i < len(p.CUs); i++ {
		p.numCUAck++
		req := CUPipelineFlushReqBuilder{}.
			WithSendTime(now).
			WithSrc(p.ToCUs).
			WithDst(p.CUs[i]).
			Build()
		p.toCUsSender.Send(req)
	}

	p.ToDriver.Retrieve(now)

	return true
}

func (p *CommandProcessor) processCUPipelineFlushRsp(
	now akita.VTimeInSec,
	rsp *CUPipelineFlushRsp,
) bool {
	p.numCUAck--

	if p.numCUAck == 0 {
		for i := 0; i < len(p.AddressTranslators); i++ {
			req := addresstranslator.AddressTranslatorFlushReqBuilder{}.
				WithSendTime(now).
				WithSrc(p.ToAddressTranslators).
				WithDst(p.AddressTranslators[i]).
				Build()
			p.toAddressTranslatorsSender.Send(req)
			p.numAddrTranslationAck++
		}
	}

	p.ToCUs.Retrieve(now)

	return true
}

func (p *CommandProcessor) processAddressTranslatorFlushRsp(
	now akita.VTimeInSec,
	cmd *addresstranslator.AddressTranslatorFlushRsp,
) bool {
	p.numAddrTranslationAck--

	if p.numAddrTranslationAck == 0 {
		for _, port := range p.L1SCaches {
			p.flushAndResetL1Cache(now, port)
		}

		for _, port := range p.L1VCaches {
			p.flushAndResetL1Cache(now, port)
		}

		for _, port := range p.L1ICaches {
			p.flushAndResetL1Cache(now, port)
		}

		for _, port := range p.L2Caches {
			p.flushAndResetL2Cache(now, port)
		}
	}

	p.ToAddressTranslators.Retrieve(now)

	return true
}

func (p *CommandProcessor) flushAndResetL1Cache(
	now akita.VTimeInSec,
	port akita.Port,
) {
	req := cache.FlushReqBuilder{}.
		WithSendTime(now).
		WithSrc(p.ToCaches).
		WithDst(port).
		PauseAfterFlushing().
		DiscardInflight().
		InvalidateAllCacheLines().
		Build()

	p.toCachesSender.Send(req)
	p.numCacheACK++
}

func (p *CommandProcessor) flushAndResetL2Cache(now akita.VTimeInSec, port akita.Port) {
	req := cache.FlushReqBuilder{}.
		WithSendTime(now).
		WithSrc(p.ToCaches).
		WithDst(port).
		PauseAfterFlushing().
		DiscardInflight().
		InvalidateAllCacheLines().
		Build()

	p.toCachesSender.Send(req)
	p.numCacheACK++
}

func (p *CommandProcessor) processCacheFlushRsp(
	now akita.VTimeInSec,
	rsp *cache.FlushRsp,
) bool {
	p.numCacheACK--
	if p.numCacheACK == 0 {
		if p.shootDownInProcess {
			return p.processCacheFlushCausedByTLBShootdown(now, rsp)
		}
		return p.processRegularCacheFlush(now, rsp)
	}

	p.ToCaches.Retrieve(now)
	return true
}

func (p *CommandProcessor) processRegularCacheFlush(
	now akita.VTimeInSec,
	flushRsp *cache.FlushRsp,
) bool {
	p.currFlushRequest.Src, p.currFlushRequest.Dst =
		p.currFlushRequest.Dst, p.currFlushRequest.Src
	p.currFlushRequest.SendTime = now

	p.toDriverSender.Send(p.currFlushRequest)

	p.ToCaches.Retrieve(now)

	return true
}

func (p *CommandProcessor) processCacheFlushCausedByTLBShootdown(
	now akita.VTimeInSec,
	flushRsp *cache.FlushRsp,
) bool {
	for i := 0; i < len(p.TLBs); i++ {
		shootDownCmd := p.currShootdownRequest
		req := tlb.TLBFlushReqBuilder{}.
			WithSendTime(now).
			WithSrc(p.ToTLBs).
			WithDst(p.TLBs[i]).
			WithPID(shootDownCmd.PID).
			WithVAddrs(shootDownCmd.VAddr).
			Build()

		p.toTLBsSender.Send(req)
		p.numTLBAck++
	}

	p.ToCaches.Retrieve(now)
	return true
}

func (p *CommandProcessor) processTLBFlushRsp(
	now akita.VTimeInSec,
	rsp *tlb.TLBFlushRsp,
) bool {
	p.numTLBAck--

	if p.numTLBAck == 0 {
		req := NewShootdownCompleteRsp(now, p.ToDriver, p.Driver)
		p.toDriverSender.Send(req)

		p.shootDownInProcess = false
	}

	p.ToTLBs.Retrieve(now)

	return true
}

func (p *CommandProcessor) processRDMARestartCommand(
	now akita.VTimeInSec,
	cmd *RDMARestartCmdFromDriver,
) bool {
	req := rdma.RDMARestartReqBuilder{}.
		WithSrc(p.ToRDMA).
		WithDst(p.RDMA).
		WithSendTime(now).
		Build()

	p.toRDMASender.Send(req)

	p.ToDriver.Retrieve(now)

	return true
}

func (p *CommandProcessor) processGPURestartReq(
	now akita.VTimeInSec,
	cmd *GPURestartReq,
) bool {
	for _, port := range p.L2Caches {
		p.restartCache(now, port)
	}
	for _, port := range p.L1ICaches {
		p.restartCache(now, port)
	}
	for _, port := range p.L1SCaches {
		p.restartCache(now, port)
	}

	for _, port := range p.L1VCaches {
		p.restartCache(now, port)
	}

	p.ToDriver.Retrieve(now)

	return true
}

func (p *CommandProcessor) restartCache(now akita.VTimeInSec, port akita.Port) {
	req := cache.RestartReqBuilder{}.
		WithSendTime(now).
		WithSrc(p.ToDispatcher).
		WithDst(port).
		Build()

	p.toCachesSender.Send(req)

	p.numCacheACK++
}

func (p *CommandProcessor) processCacheRestartRsp(
	now akita.VTimeInSec,
	rsp *cache.RestartRsp,
) bool {
	p.numCacheACK--
	if p.numCacheACK == 0 {
		for i := 0; i < len(p.TLBs); i++ {
			p.numTLBAck++

			req := tlb.TLBRestartReqBuilder{}.
				WithSendTime(now).
				WithSrc(p.ToTLBs).
				WithDst(p.TLBs[i]).
				Build()
			p.toTLBsSender.Send(req)
		}
	}

	p.ToCaches.Retrieve(now)

	return true
}

func (p *CommandProcessor) processTLBRestartRsp(
	now akita.VTimeInSec,
	rsp *tlb.TLBRestartRsp,
) bool {
	p.numTLBAck--

	if p.numTLBAck == 0 {
		for i := 0; i < len(p.AddressTranslators); i++ {
			req := addresstranslator.AddressTranslatorRestartReqBuilder{}.
				WithSendTime(now).
				WithSrc(p.ToAddressTranslators).
				WithDst(p.AddressTranslators[i]).
				Build()
			p.toAddressTranslatorsSender.Send(req)

			p.numAddrTranslationAck++
		}
	}

	p.ToTLBs.Retrieve(now)

	return true
}

func (p *CommandProcessor) processAddressTranslatorRestartRsp(
	now akita.VTimeInSec,
	rsp *addresstranslator.AddressTranslatorRestartRsp,
) bool {
	p.numAddrTranslationAck--

	if p.numAddrTranslationAck == 0 {
		for i := 0; i < len(p.CUs); i++ {
			req := CUPipelineRestartReqBuilder{}.
				WithSendTime(now).
				WithSrc(p.ToCUs).
				WithDst(p.CUs[i]).
				Build()
			p.toCUsSender.Send(req)

			p.numCUAck++
		}
	}

	p.ToAddressTranslators.Retrieve(now)

	return true
}

func (p *CommandProcessor) processCUPipelineRestartRsp(
	now akita.VTimeInSec,
	rsp *CUPipelineRestartRsp,
) bool {
	p.numCUAck--

	if p.numCUAck == 0 {
		rsp := NewGPURestartRsp(now, p.ToDriver, p.Driver)
		p.toDriverSender.Send(rsp)
	}

	p.ToCUs.Retrieve(now)

	return true
}

func (p *CommandProcessor) processPageMigrationReq(
	now akita.VTimeInSec,
	cmd *PageMigrationReqToCP,
) bool {
	req := pagemigrationcontroller.PageMigrationReqToPMCBuilder{}.
		WithSendTime(now).
		WithSrc(p.ToPMC).
		WithDst(p.PMC).
		WithPageSize(cmd.PageSize).
		WithPMCPortOfRemoteGPU(cmd.DestinationPMCPort).
		WithReadFrom(cmd.ToReadFromPhysicalAddress).
		WithWriteTo(cmd.ToWriteToPhysicalAddress).
		Build()
	p.toPMCSender.Send(req)

	p.ToDriver.Retrieve(now)

	return true
}

func (p *CommandProcessor) processPageMigrationRsp(
	now akita.VTimeInSec,
	rsp *pagemigrationcontroller.PageMigrationRspFromPMC,
) bool {
	req := NewPageMigrationRspToDriver(now, p.ToDriver, p.Driver)

	p.toDriverSender.Send(req)

	p.ToPMC.Retrieve(now)

	return true
}

func (p *CommandProcessor) processFlushCommand(
	now akita.VTimeInSec,
	cmd *FlushCommand,
) bool {
	for _, port := range p.L1ICaches {
		p.flushCache(now, port)
	}

	for _, port := range p.L1SCaches {
		p.flushCache(now, port)
	}

	for _, port := range p.L1VCaches {
		p.flushCache(now, port)
	}

	for _, port := range p.L2Caches {
		p.flushCache(now, port)
	}

	p.currFlushRequest = cmd
	if p.numCacheACK == 0 {
		p.currFlushRequest.Src, p.currFlushRequest.Dst =
			p.currFlushRequest.Dst, p.currFlushRequest.Src
		p.currFlushRequest.SendTime = now
		p.toDriverSender.Send(p.currFlushRequest)
	}

	p.ToDriver.Retrieve(now)

	return true
}

func (p *CommandProcessor) flushCache(now akita.VTimeInSec, port akita.Port) {
	flushReq := cache.FlushReqBuilder{}.
		WithSendTime(now).
		WithSrc(p.ToCaches).
		WithDst(port).
		Build()
	p.toCachesSender.Send(flushReq)
	p.numCacheACK++
}

func (p *CommandProcessor) processMemCopyReq(
	now akita.VTimeInSec,
	req akita.Msg,
) bool {
	req.Meta().Dst = p.DMAEngine
	req.Meta().Src = p.ToDispatcher
	req.Meta().SendTime = now
	p.toDispatcherSender.Send(req)
	p.ToDriver.Retrieve(now)
	return true
}

func (p *CommandProcessor) processMemCopyRsp(
	now akita.VTimeInSec,
	req akita.Msg,
) bool {
	req.Meta().Dst = p.Driver
	req.Meta().Src = p.ToDriver
	req.Meta().SendTime = now
	p.toDriverSender.Send(req)
	p.ToDispatcher.Retrieve(now)
	return true
}

// NewCommandProcessor creates a new CommandProcessor
func NewCommandProcessor(name string, engine akita.Engine) *CommandProcessor {
	c := new(CommandProcessor)
	c.TickingComponent = akita.NewTickingComponent(name, engine, 1*akita.GHz, c)

	c.kernelFixedOverheadInCycles = 1600

	unlimited := math.MaxInt32
	c.ToDriver = akita.NewLimitNumMsgPort(c, 1, name+".ToDriver")
	c.toDriverSender = akitaext.NewBufferedSender(
		c.ToDriver, util.NewBuffer(unlimited))
	c.ToDispatcher = akita.NewLimitNumMsgPort(c, 1, name+".ToDispatcher")
	c.toDispatcherSender = akitaext.NewBufferedSender(
		c.ToDispatcher, util.NewBuffer(unlimited))
	c.ToCUs = akita.NewLimitNumMsgPort(c, 1, name+".ToCUs")
	c.toCUsSender = akitaext.NewBufferedSender(
		c.ToDispatcher, util.NewBuffer(unlimited))
	c.ToTLBs = akita.NewLimitNumMsgPort(c, 1, name+".ToTLBs")
	c.toTLBsSender = akitaext.NewBufferedSender(
		c.ToDispatcher, util.NewBuffer(unlimited))
	c.ToRDMA = akita.NewLimitNumMsgPort(c, 1, name+".ToRDMA")
	c.toRDMASender = akitaext.NewBufferedSender(
		c.ToDispatcher, util.NewBuffer(unlimited))
	c.ToPMC = akita.NewLimitNumMsgPort(c, 1, name+".ToPMC")
	c.toPMCSender = akitaext.NewBufferedSender(
		c.ToDispatcher, util.NewBuffer(unlimited))
	c.ToAddressTranslators = akita.NewLimitNumMsgPort(c, 1,
		name+".ToAddressTranslators")
	c.toAddressTranslatorsSender = akitaext.NewBufferedSender(
		c.ToDispatcher, util.NewBuffer(unlimited))
	c.ToCaches = akita.NewLimitNumMsgPort(c, 1, name+".ToCaches")
	c.toCachesSender = akitaext.NewBufferedSender(
		c.ToDispatcher, util.NewBuffer(unlimited))

	c.bottomReqIDToTopReqMap = make(map[string]*LaunchKernelReq)

	return c
}

type ReplyKernelCompletionEvent struct {
	*akita.EventBase
	Req *LaunchKernelReq
}

func NewReplyKernelCompletionEvent(
	time akita.VTimeInSec,
	handler akita.Handler,
	req *LaunchKernelReq,
) *ReplyKernelCompletionEvent {
	evt := new(ReplyKernelCompletionEvent)
	evt.EventBase = akita.NewEventBase(time, handler)
	evt.Req = req
	return evt
}
