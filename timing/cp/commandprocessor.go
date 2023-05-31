package cp

import (
	"github.com/sarchlab/akita/v3/mem/cache"
	"github.com/sarchlab/akita/v3/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/mem/vm/tlb"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/protocol"
	"github.com/sarchlab/mgpusim/v3/timing/cp/internal/dispatching"
	"github.com/sarchlab/mgpusim/v3/timing/cp/internal/resource"
	"github.com/sarchlab/mgpusim/v3/timing/pagemigrationcontroller"
	"github.com/sarchlab/mgpusim/v3/timing/rdma"
)

// CommandProcessor is an Akita component that is responsible for receiving
// requests from the driver and dispatch the requests to other parts of the
// GPU.
type CommandProcessor struct {
	*sim.TickingComponent

	Dispatchers        []dispatching.Dispatcher
	DMAEngine          sim.Port
	Driver             sim.Port
	TLBs               []sim.Port
	CUs                []sim.Port
	AddressTranslators []sim.Port
	RDMA               sim.Port
	PMC                sim.Port
	L1VCaches          []sim.Port
	L1SCaches          []sim.Port
	L1ICaches          []sim.Port
	L2Caches           []sim.Port
	DRAMControllers    []*idealmemcontroller.Comp

	ToDriver                   sim.Port
	toDriverSender             sim.BufferedSender
	ToDMA                      sim.Port
	toDMASender                sim.BufferedSender
	ToCUs                      sim.Port
	toCUsSender                sim.BufferedSender
	ToTLBs                     sim.Port
	toTLBsSender               sim.BufferedSender
	ToAddressTranslators       sim.Port
	toAddressTranslatorsSender sim.BufferedSender
	ToCaches                   sim.Port
	toCachesSender             sim.BufferedSender
	ToRDMA                     sim.Port
	toRDMASender               sim.BufferedSender
	ToPMC                      sim.Port
	toPMCSender                sim.BufferedSender

	currShootdownRequest *protocol.ShootDownCommand
	currFlushRequest     *protocol.FlushReq

	numTLBs                      uint64
	numCUAck                     uint64
	numAddrTranslationFlushAck   uint64
	numAddrTranslationRestartAck uint64
	numTLBAck                    uint64
	numCacheACK                  uint64

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
	ControlPort() sim.Port
}

// RegisterCU allows the Command Processor to control the CU.
func (p *CommandProcessor) RegisterCU(cu CUInterfaceForCP) {
	p.CUs = append(p.CUs, cu.ControlPort())
	for _, d := range p.Dispatchers {
		d.RegisterCU(cu)
	}
}

// Tick ticks
func (p *CommandProcessor) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = p.sendMsgsOut(now) || madeProgress
	madeProgress = p.tickDispatchers(now) || madeProgress
	madeProgress = p.processReqFromDriver(now) || madeProgress
	madeProgress = p.processRspFromInternal(now) || madeProgress

	return madeProgress
}

func (p *CommandProcessor) sendMsgsOut(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = p.sendMsgsOutFromPort(now, p.toDriverSender) || madeProgress
	madeProgress = p.sendMsgsOutFromPort(now, p.toDMASender) || madeProgress
	madeProgress = p.sendMsgsOutFromPort(now, p.toCUsSender) || madeProgress
	madeProgress = p.sendMsgsOutFromPort(now, p.toTLBsSender) || madeProgress
	madeProgress = p.sendMsgsOutFromPort(
		now, p.toAddressTranslatorsSender) || madeProgress
	madeProgress = p.sendMsgsOutFromPort(now, p.toCachesSender) || madeProgress
	madeProgress = p.sendMsgsOutFromPort(now, p.toRDMASender) || madeProgress
	madeProgress = p.sendMsgsOutFromPort(now, p.toPMCSender) || madeProgress

	return madeProgress
}

func (p *CommandProcessor) sendMsgsOutFromPort(
	now sim.VTimeInSec,
	sender sim.BufferedSender,
) (madeProgress bool) {
	for {
		ok := sender.Tick(now)
		if ok {
			madeProgress = true
		} else {
			return madeProgress
		}
	}
}

func (p *CommandProcessor) tickDispatchers(
	now sim.VTimeInSec,
) (madeProgress bool) {
	for _, d := range p.Dispatchers {
		madeProgress = d.Tick(now) || madeProgress
	}

	return madeProgress
}

func (p *CommandProcessor) processReqFromDriver(now sim.VTimeInSec) bool {
	msg := p.ToDriver.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *protocol.LaunchKernelReq:
		return p.processLaunchKernelReq(now, req)
	case *protocol.FlushReq:
		return p.processFlushReq(now, req)
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

func (p *CommandProcessor) processRspFromInternal(now sim.VTimeInSec) bool {
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

func (p *CommandProcessor) processRspFromDMAs(now sim.VTimeInSec) bool {
	msg := p.ToDMA.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *sim.GeneralRsp:
		return p.processMemCopyRsp(now, req)
	}

	panic("never")
}

func (p *CommandProcessor) processRspFromRDMAs(now sim.VTimeInSec) bool {
	msg := p.ToRDMA.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *rdma.DrainRsp:
		return p.processRDMADrainRsp(now, req)
	case *rdma.RestartRsp:
		return p.processRDMARestartRsp(now, req)
	}

	panic("never")
}

func (p *CommandProcessor) processRspFromCUs(now sim.VTimeInSec) bool {
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

func (p *CommandProcessor) processRspFromCaches(now sim.VTimeInSec) bool {
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

func (p *CommandProcessor) processRspFromATs(now sim.VTimeInSec) bool {
	item := p.ToAddressTranslators.Peek()
	if item == nil {
		return false
	}

	msg := item.(*mem.ControlMsg)

	if p.numAddrTranslationFlushAck > 0 {
		return p.processAddressTranslatorFlushRsp(now, msg)
	} else if p.numAddrTranslationRestartAck > 0 {
		return p.processAddressTranslatorRestartRsp(now, msg)
	}

	panic("never")
}

func (p *CommandProcessor) processRspFromTLBs(now sim.VTimeInSec) bool {
	msg := p.ToTLBs.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *tlb.FlushRsp:
		return p.processTLBFlushRsp(now, req)
	case *tlb.RestartRsp:
		return p.processTLBRestartRsp(now, req)
	}

	panic("never")
}

func (p *CommandProcessor) processRspFromPMC(now sim.VTimeInSec) bool {
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
	now sim.VTimeInSec,
	req *protocol.LaunchKernelReq,
) bool {
	d := p.findAvailableDispatcher()

	if d == nil {
		return false
	}

	d.StartDispatching(req)
	p.ToDriver.Retrieve(now)

	tracing.TraceReqReceive(req, p)
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
	now sim.VTimeInSec,
	cmd *protocol.RDMADrainCmdFromDriver,
) bool {
	req := rdma.DrainReqBuilder{}.
		WithSendTime(now).
		WithSrc(p.ToRDMA).
		WithDst(p.RDMA).
		Build()

	p.toRDMASender.Send(req)
	p.ToDriver.Retrieve(now)

	return true
}

func (p *CommandProcessor) processRDMADrainRsp(
	now sim.VTimeInSec,
	rsp *rdma.DrainRsp,
) bool {
	req := protocol.NewRDMADrainRspToDriver(now, p.ToDriver, p.Driver)

	p.toDriverSender.Send(req)
	p.ToRDMA.Retrieve(now)

	return true
}

func (p *CommandProcessor) processShootdownCommand(
	now sim.VTimeInSec,
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
	now sim.VTimeInSec,
	rsp *protocol.CUPipelineFlushRsp,
) bool {
	p.numCUAck--

	if p.numCUAck == 0 {
		for i := 0; i < len(p.AddressTranslators); i++ {
			req := mem.ControlMsgBuilder{}.
				WithSendTime(now).
				WithSrc(p.ToAddressTranslators).
				WithDst(p.AddressTranslators[i]).
				ToDiscardTransactions().
				Build()
			p.toAddressTranslatorsSender.Send(req)
			p.numAddrTranslationFlushAck++
		}
	}

	p.ToCUs.Retrieve(now)

	return true
}

func (p *CommandProcessor) processAddressTranslatorFlushRsp(
	now sim.VTimeInSec,
	msg *mem.ControlMsg,
) bool {
	p.numAddrTranslationFlushAck--

	if p.numAddrTranslationFlushAck == 0 {
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
	now sim.VTimeInSec,
	port sim.Port,
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

func (p *CommandProcessor) flushAndResetL2Cache(now sim.VTimeInSec, port sim.Port) {
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
	now sim.VTimeInSec,
	rsp *cache.FlushRsp,
) bool {
	p.numCacheACK--
	p.ToCaches.Retrieve(now)

	if p.numCacheACK == 0 {
		if p.shootDownInProcess {
			return p.processCacheFlushCausedByTLBShootdown(now, rsp)
		}
		return p.processRegularCacheFlush(now, rsp)
	}

	return true
}

func (p *CommandProcessor) processRegularCacheFlush(
	now sim.VTimeInSec,
	flushRsp *cache.FlushRsp,
) bool {
	rsp := sim.GeneralRspBuilder{}.
		WithSendTime(now).
		WithSrc(p.ToDriver).
		WithDst(p.Driver).
		WithOriginalReq(p.currFlushRequest).
		Build()

	p.toDriverSender.Send(rsp)

	tracing.TraceReqComplete(p.currFlushRequest, p)
	p.currFlushRequest = nil

	return true
}

func (p *CommandProcessor) processCacheFlushCausedByTLBShootdown(
	now sim.VTimeInSec,
	flushRsp *cache.FlushRsp,
) bool {
	p.currFlushRequest = nil

	for i := 0; i < len(p.TLBs); i++ {
		shootDownCmd := p.currShootdownRequest
		req := tlb.FlushReqBuilder{}.
			WithSendTime(now).
			WithSrc(p.ToTLBs).
			WithDst(p.TLBs[i]).
			WithPID(shootDownCmd.PID).
			WithVAddrs(shootDownCmd.VAddr).
			Build()

		p.toTLBsSender.Send(req)
		p.numTLBAck++
	}

	return true
}

func (p *CommandProcessor) processTLBFlushRsp(
	now sim.VTimeInSec,
	rsp *tlb.FlushRsp,
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
	now sim.VTimeInSec,
	cmd *protocol.RDMARestartCmdFromDriver,
) bool {
	req := rdma.RestartReqBuilder{}.
		WithSrc(p.ToRDMA).
		WithDst(p.RDMA).
		WithSendTime(now).
		Build()

	p.toRDMASender.Send(req)

	p.ToDriver.Retrieve(now)

	return true
}

func (p *CommandProcessor) processRDMARestartRsp(now sim.VTimeInSec, rsp *rdma.RestartRsp) bool {
	req := protocol.NewRDMARestartRspToDriver(now, p.ToDriver, p.Driver)
	p.toDriverSender.Send(req)
	p.ToRDMA.Retrieve(now)

	return true
}

func (p *CommandProcessor) processGPURestartReq(
	now sim.VTimeInSec,
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

func (p *CommandProcessor) restartCache(now sim.VTimeInSec, port sim.Port) {
	req := cache.RestartReqBuilder{}.
		WithSendTime(now).
		WithSrc(p.ToCaches).
		WithDst(port).
		Build()

	p.toCachesSender.Send(req)

	p.numCacheACK++
}

func (p *CommandProcessor) processCacheRestartRsp(
	now sim.VTimeInSec,
	rsp *cache.RestartRsp,
) bool {
	p.numCacheACK--
	if p.numCacheACK == 0 {
		for i := 0; i < len(p.TLBs); i++ {
			p.numTLBAck++

			req := tlb.RestartReqBuilder{}.
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
	now sim.VTimeInSec,
	rsp *tlb.RestartRsp,
) bool {
	p.numTLBAck--

	if p.numTLBAck == 0 {
		for i := 0; i < len(p.AddressTranslators); i++ {
			req := mem.ControlMsgBuilder{}.
				WithSendTime(now).
				WithSrc(p.ToAddressTranslators).
				WithDst(p.AddressTranslators[i]).
				ToRestart().
				Build()
			p.toAddressTranslatorsSender.Send(req)

			// fmt.Printf("Restarting %s\n", p.AddressTranslators[i].Name())

			p.numAddrTranslationRestartAck++
		}
	}

	p.ToTLBs.Retrieve(now)

	return true
}

func (p *CommandProcessor) processAddressTranslatorRestartRsp(
	now sim.VTimeInSec,
	rsp *mem.ControlMsg,
) bool {
	p.numAddrTranslationRestartAck--

	if p.numAddrTranslationRestartAck == 0 {
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
	now sim.VTimeInSec,
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
	now sim.VTimeInSec,
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
	now sim.VTimeInSec,
	rsp *pagemigrationcontroller.PageMigrationRspFromPMC,
) bool {
	req := protocol.NewPageMigrationRspToDriver(now, p.ToDriver, p.Driver)

	p.toDriverSender.Send(req)

	p.ToPMC.Retrieve(now)

	return true
}

func (p *CommandProcessor) processFlushReq(
	now sim.VTimeInSec,
	req *protocol.FlushReq,
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

	p.currFlushRequest = req
	if p.numCacheACK == 0 {
		rsp := sim.GeneralRspBuilder{}.
			WithSendTime(now).
			WithSrc(p.ToDriver).
			WithDst(p.Driver).
			WithOriginalReq(req).
			Build()
		p.toDriverSender.Send(rsp)
	}

	p.ToDriver.Retrieve(now)

	tracing.TraceReqReceive(req, p)

	return true
}

func (p *CommandProcessor) flushCache(now sim.VTimeInSec, port sim.Port) {
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
	cloned.ID = sim.GetIDGenerator().Generate()
	p.bottomMemCopyH2DReqIDToTopReqMap[cloned.ID] = req
	return &cloned
}

func (p *CommandProcessor) cloneMemCopyD2HReq(
	req *protocol.MemCopyD2HReq,
) *protocol.MemCopyD2HReq {
	cloned := *req
	cloned.ID = sim.GetIDGenerator().Generate()
	p.bottomMemCopyD2HReqIDToTopReqMap[cloned.ID] = req
	return &cloned
}

func (p *CommandProcessor) processMemCopyReq(
	now sim.VTimeInSec,
	req sim.Msg,
) bool {
	if p.numCacheACK > 0 {
		return false
	}

	var cloned sim.Msg
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

	tracing.TraceReqReceive(req, p)
	tracing.TraceReqInitiate(cloned, p, tracing.MsgIDAtReceiver(req, p))

	return true
}

func (p *CommandProcessor) findAndRemoveOriginalMemCopyRequest(
	rsp sim.Rsp,
) sim.Msg {
	rspTo := rsp.GetRspTo()

	originalH2DReq, ok := p.bottomMemCopyH2DReqIDToTopReqMap[rspTo]
	if ok {
		delete(p.bottomMemCopyH2DReqIDToTopReqMap, rspTo)
		return originalH2DReq
	}

	originalD2HReq, ok := p.bottomMemCopyD2HReqIDToTopReqMap[rspTo]
	if ok {
		delete(p.bottomMemCopyD2HReqIDToTopReqMap, rspTo)
		return originalD2HReq
	}

	panic("never")
}

func (p *CommandProcessor) processMemCopyRsp(
	now sim.VTimeInSec,
	req sim.Rsp,
) bool {
	originalReq := p.findAndRemoveOriginalMemCopyRequest(req)

	rsp := sim.GeneralRspBuilder{}.
		WithDst(p.Driver).
		WithSrc(p.ToDriver).
		WithSendTime(now).
		WithOriginalReq(originalReq).
		Build()

	p.toDriverSender.Send(rsp)
	p.ToDMA.Retrieve(now)

	tracing.TraceReqComplete(originalReq, p)
	tracing.TraceReqFinalize(req, p)

	return true
}
