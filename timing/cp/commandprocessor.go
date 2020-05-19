package cp

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/idealmemcontroller"
	"gitlab.com/akita/mem/vm/addresstranslator"
	"gitlab.com/akita/mem/vm/tlb"
	"gitlab.com/akita/mgpusim/pagemigrationcontroller"
	"gitlab.com/akita/mgpusim/protocol"
	"gitlab.com/akita/mgpusim/rdma"
	"gitlab.com/akita/mgpusim/timing/cp/internal/dispatching"
	"gitlab.com/akita/mgpusim/timing/cp/internal/resource"
	"gitlab.com/akita/util/akitaext"
	"gitlab.com/akita/util/tracing"
)

// CommandProcessor is an Akita component that is responsible for receiving
// requests from the driver and dispatch the requests to other parts of the
// GPU.
type CommandProcessor struct {
	*akita.TickingComponent

	Dispatchers        []dispatching.Dispatcher
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
	ToDMA                      akita.Port
	toDMASender                akitaext.BufferedSender
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

	currShootdownRequest *protocol.ShootDownCommand
	currFlushRequest     *protocol.FlushCommand

	numTLBs               uint64
	numCUAck              uint64
	numAddrTranslationAck uint64
	numTLBAck             uint64
	numCacheACK           uint64

	shootDownInProcess bool

	bottomKernelLaunchReqIDToTopReqMap map[string]*protocol.LaunchKernelReq
	bottomMemCopyH2DReqIDToTopReqMap   map[string]*protocol.MemCopyH2DReq
	bottomMemCopyD2HReqIDToTopReqMap   map[string]*protocol.MemCopyD2HReq
}

// CUInterfaceForCP defines the interface that a CP requires from CU.
type CUInterfaceForCP interface {
	resource.DispatchableCU

	// ControlPort returns a port on the CU that the CP can send controlling
	// messages to.
	ControlPort() akita.Port
}

// RegisterCU allows the Command Processor to control the CU.
func (p *CommandProcessor) RegisterCU(cu CUInterfaceForCP) {
	p.CUs = append(p.CUs, cu.ControlPort())
	for _, d := range p.Dispatchers {
		d.RegisterCU(cu)
	}
}

func (p *CommandProcessor) Tick(now akita.VTimeInSec) bool {
	madeProgress := false

	madeProgress = p.sendMsgsOut(now) || madeProgress
	madeProgress = p.tickDispatchers(now) || madeProgress
	madeProgress = p.processReqFromDriver(now) || madeProgress
	madeProgress = p.processRspFromInternal(now) || madeProgress

	return madeProgress
}

func (p *CommandProcessor) sendMsgsOut(now akita.VTimeInSec) bool {
	madeProgress := false

	madeProgress = p.toDriverSender.Tick(now) || madeProgress
	madeProgress = p.toDMASender.Tick(now) || madeProgress
	madeProgress = p.toCUsSender.Tick(now) || madeProgress
	madeProgress = p.toTLBsSender.Tick(now) || madeProgress
	madeProgress = p.toAddressTranslatorsSender.Tick(now) || madeProgress
	madeProgress = p.toCachesSender.Tick(now) || madeProgress
	madeProgress = p.toRDMASender.Tick(now) || madeProgress
	madeProgress = p.toPMCSender.Tick(now) || madeProgress

	return madeProgress
}

func (p *CommandProcessor) tickDispatchers(
	now akita.VTimeInSec,
) (madeProgress bool) {
	for _, d := range p.Dispatchers {
		madeProgress = d.Tick(now) || madeProgress
	}

	return madeProgress
}

func (p *CommandProcessor) processReqFromDriver(now akita.VTimeInSec) bool {
	msg := p.ToDriver.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *protocol.LaunchKernelReq:
		return p.processLaunchKernelReq(now, req)
	case *protocol.FlushCommand:
		return p.processFlushCommand(now, req)
	case *protocol.MemCopyD2HReq, *protocol.MemCopyH2DReq:
		return p.processMemCopyReq(now, req)
	case *protocol.RDMADrainCmdFromDriver:
		return p.processRDMADrainCmd(now, req)
	case *protocol.RDMARestartCmdFromDriver:
		return p.processRDMARestartCommand(now, req)
	case *protocol.ShootDownCommand:
		return p.processShootdownCommand(now, req)
	case *protocol.GPURestartReq:
		return p.processGPURestartReq(now, req)
	case *protocol.PageMigrationReqToCP:
		return p.processPageMigrationReq(now, req)
	}

	panic("never")
}

func (p *CommandProcessor) processRspFromInternal(now akita.VTimeInSec) bool {
	madeProgress := false

	madeProgress = p.processRspFromDMAs(now) || madeProgress
	madeProgress = p.processRspFromRDMAs(now) || madeProgress
	madeProgress = p.processRspFromCUs(now) || madeProgress
	madeProgress = p.processRspFromATs(now) || madeProgress
	madeProgress = p.processRspFromCaches(now) || madeProgress
	madeProgress = p.processRspFromTLBs(now) || madeProgress
	madeProgress = p.processRspFromPMC(now) || madeProgress

	return madeProgress
}

func (p *CommandProcessor) processRspFromDMAs(now akita.VTimeInSec) bool {
	msg := p.ToDMA.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *protocol.MemCopyD2HReq, *protocol.MemCopyH2DReq:
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
	case *rdma.RDMARestartRsp:
		return p.processRDMARestartRsp(now, req)
	}

	panic("never")
}

func (p *CommandProcessor) processRspFromCUs(now akita.VTimeInSec) bool {
	msg := p.ToCUs.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *protocol.CUPipelineFlushRsp:
		return p.processCUPipelineFlushRsp(now, req)
	case *protocol.CUPipelineRestartRsp:
		return p.processCUPipelineRestartRsp(now, req)
	}

	return false
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
	req *protocol.LaunchKernelReq,
) bool {
	d := p.findAvailableDispatcher()

	if d == nil {
		return false
	}

	d.StartDispatching(req)
	p.ToDriver.Retrieve(now)

	tracing.TraceReqReceive(req, now, p)
	// tracing.TraceReqInitiate(&reqToBottom, now, p,
	// 	tracing.MsgIDAtReceiver(req, p))

	return true
}

func (p *CommandProcessor) findAvailableDispatcher() dispatching.Dispatcher {
	for _, d := range p.Dispatchers {
		if !d.IsDispatching() {
			return d
		}
	}

	return nil
}
func (p *CommandProcessor) processRDMADrainCmd(
	now akita.VTimeInSec,
	cmd *protocol.RDMADrainCmdFromDriver,
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
	req := protocol.NewRDMADrainRspToDriver(now, p.ToDriver, p.Driver)

	p.toDriverSender.Send(req)
	p.ToRDMA.Retrieve(now)

	return true
}

func (p *CommandProcessor) processShootdownCommand(
	now akita.VTimeInSec,
	cmd *protocol.ShootDownCommand,
) bool {
	if p.shootDownInProcess == true {
		return false
	}

	p.currShootdownRequest = cmd
	p.shootDownInProcess = true

	for i := 0; i < len(p.CUs); i++ {
		p.numCUAck++
		req := protocol.CUPipelineFlushReqBuilder{}.
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
	rsp *protocol.CUPipelineFlushRsp,
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
		req := protocol.NewShootdownCompleteRsp(now, p.ToDriver, p.Driver)
		p.toDriverSender.Send(req)

		p.shootDownInProcess = false
	}

	p.ToTLBs.Retrieve(now)

	return true
}

func (p *CommandProcessor) processRDMARestartCommand(
	now akita.VTimeInSec,
	cmd *protocol.RDMARestartCmdFromDriver,
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

func (p *CommandProcessor) processRDMARestartRsp(now akita.VTimeInSec, rsp *rdma.RDMARestartRsp) bool {
	req := protocol.NewRDMARestartRspToDriver(now, p.ToDriver, p.Driver)
	p.toDriverSender.Send(req)
	p.ToRDMA.Retrieve(now)

	return true
}

func (p *CommandProcessor) processGPURestartReq(
	now akita.VTimeInSec,
	cmd *protocol.GPURestartReq,
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
		WithSrc(p.ToCaches).
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
			req := protocol.CUPipelineRestartReqBuilder{}.
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
	rsp *protocol.CUPipelineRestartRsp,
) bool {
	p.numCUAck--

	if p.numCUAck == 0 {
		rsp := protocol.NewGPURestartRsp(now, p.ToDriver, p.Driver)
		p.toDriverSender.Send(rsp)
	}

	p.ToCUs.Retrieve(now)

	return true
}

func (p *CommandProcessor) processPageMigrationReq(
	now akita.VTimeInSec,
	cmd *protocol.PageMigrationReqToCP,
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
	req := protocol.NewPageMigrationRspToDriver(now, p.ToDriver, p.Driver)

	p.toDriverSender.Send(req)

	p.ToPMC.Retrieve(now)

	return true
}

func (p *CommandProcessor) processFlushCommand(
	now akita.VTimeInSec,
	cmd *protocol.FlushCommand,
) bool {
	if p.numCacheACK > 0 {
		return false
	}

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

func (p *CommandProcessor) cloneMemCopyH2DReq(
	req *protocol.MemCopyH2DReq,
) *protocol.MemCopyH2DReq {
	cloned := *req
	cloned.ID = akita.GetIDGenerator().Generate()
	p.bottomMemCopyH2DReqIDToTopReqMap[cloned.ID] = req
	return &cloned
}

func (p *CommandProcessor) cloneMemCopyD2HReq(
	req *protocol.MemCopyD2HReq,
) *protocol.MemCopyD2HReq {
	cloned := *req
	cloned.ID = akita.GetIDGenerator().Generate()
	p.bottomMemCopyD2HReqIDToTopReqMap[cloned.ID] = req
	return &cloned
}

func (p *CommandProcessor) processMemCopyReq(
	now akita.VTimeInSec,
	req akita.Msg,
) bool {
	var cloned akita.Msg
	switch req := req.(type) {
	case *protocol.MemCopyH2DReq:
		cloned = p.cloneMemCopyH2DReq(req)
	case *protocol.MemCopyD2HReq:
		cloned = p.cloneMemCopyD2HReq(req)
	default:
		panic("unknown type")
	}

	cloned.Meta().Dst = p.DMAEngine
	cloned.Meta().Src = p.ToDMA
	cloned.Meta().SendTime = now

	p.toDMASender.Send(cloned)
	p.ToDriver.Retrieve(now)

	tracing.TraceReqReceive(req, now, p)
	tracing.TraceReqInitiate(cloned, now, p, tracing.MsgIDAtReceiver(req, p))

	return true
}

func (p *CommandProcessor) findAndRemoveOriginalMemCopyRequest(
	rsp akita.Msg,
) akita.Msg {
	switch rsp := rsp.(type) {
	case *protocol.MemCopyH2DReq:
		origionalReq := p.bottomMemCopyH2DReqIDToTopReqMap[rsp.ID]
		delete(p.bottomMemCopyH2DReqIDToTopReqMap, rsp.ID)
		return origionalReq
	case *protocol.MemCopyD2HReq:
		originalReq := p.bottomMemCopyD2HReqIDToTopReqMap[rsp.ID]
		delete(p.bottomMemCopyD2HReqIDToTopReqMap, rsp.ID)
		return originalReq
	default:
		panic("unknown type")
	}
}

func (p *CommandProcessor) processMemCopyRsp(
	now akita.VTimeInSec,
	req akita.Msg,
) bool {
	originalReq := p.findAndRemoveOriginalMemCopyRequest(req)
	originalReq.Meta().Dst = p.Driver
	originalReq.Meta().Src = p.ToDriver
	originalReq.Meta().SendTime = now
	p.toDriverSender.Send(originalReq)
	p.ToDMA.Retrieve(now)

	tracing.TraceReqComplete(originalReq, now, p)
	tracing.TraceReqFinalize(req, now, p)

	return true
}
