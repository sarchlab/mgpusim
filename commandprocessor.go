package gcn3

import (
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/pagemigrationcontroller"
	"gitlab.com/akita/gcn3/rdma"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/idealmemcontroller"
	"gitlab.com/akita/mem/vm/addresstranslator"
	"gitlab.com/akita/mem/vm/tlb"
	"gitlab.com/akita/util/tracing"
)

// CommandProcessor is an Akita component that is responsible for receiving
// requests from the driver and dispatch the requests to other parts of the
// GPU.
//
//     ToDriver <=> Receive request and send feedback to the driver
//     ToDispatcher <=> Dispatcher of compute kernels
type CommandProcessor struct {
	*akita.ComponentBase

	engine akita.Engine
	Freq   akita.Freq

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

	ToDriver     akita.Port
	ToDispatcher akita.Port

	ToCUs                akita.Port
	ToTLBs               akita.Port
	ToAddressTranslators akita.Port
	ToCaches             akita.Port
	ToRDMA               akita.Port
	ToPMC                akita.Port

	kernelFixedOverheadInCycles int

	curShootdownRequest *ShootDownCommand
	currFlushRequest    *FlushCommand

	numCUs  uint64
	numTLBs uint64

	numCUAck              uint64
	numAddrTranslationAck uint64
	numTLBAck             uint64
	numCacheACK           uint64

	shootDownInProcess bool

	bottomReqIDToTopReqMap map[string]*LaunchKernelReq
}

func (p *CommandProcessor) NotifyRecv(
	now akita.VTimeInSec,
	port akita.Port,
) {
	req := port.Retrieve(now)
	akita.ProcessMsgAsEvent(req, p.engine, p.Freq)
}

func (p *CommandProcessor) NotifyPortFree(
	now akita.VTimeInSec,
	port akita.Port,
) {
	//panic("implement me")
}

// Handle processes the events that is scheduled for the CommandProcessor
func (p *CommandProcessor) Handle(e akita.Event) error {
	p.Lock()
	defer p.Unlock()

	switch req := e.(type) {
	case *LaunchKernelReq:
		return p.processLaunchKernelReq(req)
	case *ReplyKernelCompletionEvent:
		return p.handleReplyKernelCompletionEvent(req)
	case *FlushCommand:
		return p.handleFlushCommand(req)
	case *cache.FlushRsp:
		return p.handleCacheFlushRsp(req)
	case *MemCopyD2HReq:
		return p.processMemCopyReq(req)
	case *MemCopyH2DReq:
		return p.processMemCopyReq(req)
	// case *mem.InvalidDoneRsp:
	// Do nothing
	case *RDMADrainCmdFromDriver:
		return p.handleRDMADrainCommand(req)
	case *rdma.RDMADrainRsp:
		return p.handleRDMADrainRsp(req)
	case *ShootDownCommand:
		return p.handleShootdownCommand(req)
	case *CUPipelineFlushRsp:
		return p.handleCUPipelineFlushRsp(req)
	case *addresstranslator.AddressTranslatorFlushRsp:
		return p.handleAddressTranslatorFlushRsp(req)
	case *tlb.TLBFlushRsp:
		return p.handleTLBFlushRsp(req)
	case *RDMARestartCmdFromDriver:
		return p.handleRDMARestartCommand(req)
	case *GPURestartReq:
		return p.handleGPURestartReq(req)
	case *cache.RestartRsp:
		return p.handleCacheRestartRsp(req)
	case *tlb.TLBRestartRsp:
		return p.handleTLBRestartRsp(req)
	case *addresstranslator.AddressTranslatorRestartRsp:
		return p.handleAddressTranslatorRestartRsp(req)
	case *CUPipelineRestartRsp:
		return p.handleCUPipelineRestartRsp(req)
	case *PageMigrationReqToCP:
		return p.handlePageMigrationReq(req)
	case *pagemigrationcontroller.PageMigrationRspFromPMC:
		return p.handlePageMigrationRsp(req)

	default:
		log.Panicf("cannot process request %s", reflect.TypeOf(req))
	}
	return nil
}

func (p *CommandProcessor) processLaunchKernelReq(
	req *LaunchKernelReq,
) error {
	now := req.Time()
	if req.Src == p.Driver {
		reqToBottom := NewLaunchKernelReq(now, p.ToDispatcher, p.Dispatcher)
		reqToBottom.PID = req.PID
		reqToBottom.Packet = req.Packet
		reqToBottom.PacketAddress = req.PacketAddress
		reqToBottom.HsaCo = req.HsaCo
		reqToBottom.SendTime = now
		p.ToDispatcher.Send(reqToBottom)
		p.bottomReqIDToTopReqMap[reqToBottom.ID] = req
		tracing.TraceReqReceive(req, now, p)
		tracing.TraceReqInitiate(reqToBottom, now, p,
			tracing.MsgIDAtReceiver(req, p))
	} else if req.Src == p.Dispatcher {
		evt := NewReplyKernelCompletionEvent(
			p.Freq.NCyclesLater(p.kernelFixedOverheadInCycles, now),
			p, req,
		)
		p.engine.Schedule(evt)
	} else {
		log.Panic("The request sent to the command processor has unknown src")
	}
	return nil
}

func (p *CommandProcessor) handleReplyKernelCompletionEvent(evt *ReplyKernelCompletionEvent) error {
	now := evt.Time()

	req := evt.Req
	originalReq := p.bottomReqIDToTopReqMap[req.ID]
	originalReq.SendTime = now
	originalReq.Src, originalReq.Dst = originalReq.Dst, originalReq.Src
	p.ToDriver.Send(originalReq)
	tracing.TraceReqFinalize(req, now, p)
	tracing.TraceReqComplete(originalReq, now, p)
	return nil
}

func (p *CommandProcessor) handleRDMADrainCommand(cmd *RDMADrainCmdFromDriver) error {
	now := cmd.Time()
	req := rdma.RDMADrainReqBuilder{}.
		WithSendTime(now).
		WithSrc(p.ToRDMA).
		WithDst(p.RDMA).
		Build()
	err := p.ToRDMA.Send(req)
	if err != nil {
		log.Panicf("failed to send drain request to RDMA")
	}
	return nil
}

func (p *CommandProcessor) handleRDMADrainRsp(cmd *rdma.RDMADrainRsp) error {
	now := cmd.Time()
	req := NewRDMADrainRspToDriver(now, p.ToDriver, p.Driver)
	err := p.ToDriver.Send(req)
	if err != nil {
		log.Panicf("failed to send drain complete rsp to driver")
	}
	return nil
}

func (p *CommandProcessor) handleShootdownCommand(cmd *ShootDownCommand) error {

	if p.shootDownInProcess == true {
		return nil
	}

	p.curShootdownRequest = cmd
	p.shootDownInProcess = true

	now := cmd.Time()
	for i := 0; i < len(p.CUs); i++ {
		p.numCUAck++
		req := CUPipelineFlushReqBuilder{}.
			WithSendTime(now).
			WithSrc(p.ToCUs).
			WithDst(p.CUs[i]).
			Build()
		err := p.ToCUs.Send(req)
		if err != nil {
			log.Panicf("failed to send pipeline flush request to CU")
		}

	}

	return nil

}

func (p *CommandProcessor) handleCUPipelineFlushRsp(cmd *CUPipelineFlushRsp) error {
	now := cmd.Time()
	p.numCUAck--

	if p.numCUAck == 0 {

		for i := 0; i < len(p.AddressTranslators); i++ {
			req := addresstranslator.AddressTranslatorFlushReqBuilder{}.
				WithSendTime(now).
				WithSrc(p.ToAddressTranslators).
				WithDst(p.AddressTranslators[i]).
				Build()
			err := p.ToAddressTranslators.Send(req)
			p.numAddrTranslationAck++
			if err != nil {
				log.Panicf("failed to send flush to Address translator")
			}
		}
	}
	return nil
}

func (p *CommandProcessor) handleAddressTranslatorFlushRsp(cmd *addresstranslator.AddressTranslatorFlushRsp) error {
	now := cmd.Time()
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

	return nil
}

func (p *CommandProcessor) flushAndResetL1Cache(now akita.VTimeInSec, port akita.Port) {
	req := cache.FlushReqBuilder{}.
		WithSendTime(now).
		WithSrc(p.ToCaches).
		WithDst(port).
		PauseAfterFlushing().
		DiscardInflight().
		InvalidateAllCacheLines().
		Build()

	err := p.ToCaches.Send(req)
	if err != nil {
		log.Panicf("Failed to send reset request")
	}

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

	err := p.ToCaches.Send(req)
	if err != nil {
		log.Panicf("Failed to send reset request")
	}
	p.numCacheACK++

}

func (p *CommandProcessor) handleCacheFlushRsp(
	req *cache.FlushRsp,
) error {
	p.numCacheACK--
	if p.numCacheACK == 0 {
		if p.shootDownInProcess {
			for i := 0; i < len(p.TLBs); i++ {
				now := req.Time()
				shootDownCmd := p.curShootdownRequest
				req := tlb.TLBFlushReqBuilder{}.
					WithSendTime(now).
					WithSrc(p.ToTLBs).
					WithDst(p.TLBs[i]).
					WithPID(shootDownCmd.PID).
					WithVAddrs(shootDownCmd.VAddr).
					Build()
				err := p.ToTLBs.Send(req)
				p.numTLBAck++
				if err != nil {
					log.Panicf("failed to send shootdown request to TLBs")
				}
			}

		} else {
			p.currFlushRequest.Src, p.currFlushRequest.Dst =
				p.currFlushRequest.Dst, p.currFlushRequest.Src
			p.currFlushRequest.SendTime = req.Time()
			err := p.ToDriver.Send(p.currFlushRequest)
			if err != nil {
				panic("send failed")
			}
		}
	}

	return nil
}

func (p *CommandProcessor) handleTLBFlushRsp(cmd *tlb.TLBFlushRsp) error {
	now := cmd.Time()
	p.numTLBAck--

	if p.numTLBAck == 0 {
		req := NewShootdownCompleteRsp(now, p.ToDriver, p.Driver)
		err := p.ToDriver.Send(req)
		if err != nil {
			log.Panicf("Failed to send shootdown complete ack to driver")
		}
		p.shootDownInProcess = false
	}
	return nil
}

func (p *CommandProcessor) handleRDMARestartCommand(cmd *RDMARestartCmdFromDriver) error {
	now := cmd.Time()

	req := rdma.RDMARestartReqBuilder{}.
		WithSrc(p.ToRDMA).
		WithDst(p.RDMA).
		WithSendTime(now).
		Build()

	err := p.ToRDMA.Send(req)

	if err != nil {
		log.Panicf("Failed to send restart req to RDMA")
	}

	return nil
}

func (p *CommandProcessor) handleGPURestartReq(cmd *GPURestartReq) error {
	now := cmd.Time()
	for _, port := range p.L2Caches {
		p.restartL2Cache(now, port)
	}
	for _, port := range p.L1ICaches {
		p.restartL1Cache(now, port)
	}
	for _, port := range p.L1SCaches {
		p.restartL1Cache(now, port)
	}

	for _, port := range p.L1VCaches {
		p.restartL1Cache(now, port)
	}

	return nil
}

func (p *CommandProcessor) restartL2Cache(now akita.VTimeInSec, port akita.Port) {
	req := cache.RestartReqBuilder{}.
		WithSendTime(now).
		WithSrc(p.ToDispatcher).
		WithDst(port).
		Build()
	err := p.ToCaches.Send(req)
	p.numCacheACK++
	if err != nil {
		log.Panicf("Failed to send restart request")
	}
}

func (p *CommandProcessor) restartL1Cache(now akita.VTimeInSec, port akita.Port) {
	req := cache.RestartReqBuilder{}.
		WithSendTime(now).
		WithSrc(p.ToDispatcher).
		WithDst(port).
		Build()
	err := p.ToCaches.Send(req)
	p.numCacheACK++
	if err != nil {
		log.Panicf("Failed to send restart request")
	}
}
func (p *CommandProcessor) handleCacheRestartRsp(rsp *cache.RestartRsp) error {
	now := rsp.Time()
	p.numCacheACK--
	if p.numCacheACK == 0 {
		for i := 0; i < len(p.TLBs); i++ {
			p.numTLBAck++
			req := tlb.TLBRestartReqBuilder{}.
				WithSendTime(now).
				WithSrc(p.ToTLBs).
				WithDst(p.TLBs[i]).
				Build()
			err := p.ToTLBs.Send(req)

			if err != nil {
				log.Panicf("Failed to send restart req to TLB %s \n ", req.Dst.Component().Name())
			}
		}
	}
	return nil
}

func (p *CommandProcessor) handleTLBRestartRsp(rsp *tlb.TLBRestartRsp) error {
	now := rsp.Time()
	p.numTLBAck--

	if p.numTLBAck == 0 {
		for i := 0; i < len(p.AddressTranslators); i++ {
			req := addresstranslator.AddressTranslatorRestartReqBuilder{}.
				WithSendTime(now).
				WithSrc(p.ToAddressTranslators).
				WithDst(p.AddressTranslators[i]).
				Build()
			err := p.ToAddressTranslators.Send(req)
			p.numAddrTranslationAck++
			if err != nil {
				log.Panicf("Failed to send restart req to Address translation units")
			}

		}
	}
	return nil
}

func (p *CommandProcessor) handleAddressTranslatorRestartRsp(rsp *addresstranslator.AddressTranslatorRestartRsp) error {
	now := rsp.Time()
	p.numAddrTranslationAck--

	if p.numAddrTranslationAck == 0 {
		for i := 0; i < len(p.CUs); i++ {
			req := CUPipelineRestartReqBuilder{}.
				WithSendTime(now).
				WithSrc(p.ToCUs).
				WithDst(p.CUs[i]).
				Build()
			err := p.ToCUs.Send(req)
			p.numCUAck++
			if err != nil {
				log.Panicf("Failed to send restart req to CU")
			}

		}
	}
	return nil
}
func (p *CommandProcessor) handleCUPipelineRestartRsp(rsp *CUPipelineRestartRsp) error {
	now := rsp.Time()
	p.numCUAck--

	if p.numCUAck == 0 {
		rsp := NewGPURestartRsp(now, p.ToDriver, p.Driver)
		err := p.ToDriver.Send(rsp)
		if err != nil {
			log.Panicf("Failed to send restart rsp to driver")
		}

	}
	return nil
}

func (p *CommandProcessor) handlePageMigrationReq(cmd *PageMigrationReqToCP) error {
	now := cmd.Time()

	req := pagemigrationcontroller.PageMigrationReqToPMCBuilder{}.
		WithSendTime(now).
		WithSrc(p.ToPMC).
		WithDst(p.PMC).
		WithPageSize(cmd.PageSize).
		WithPMCPortOfRemoteGPU(cmd.DestinationPMCPort).
		WithReadFrom(cmd.ToReadFromPhysicalAddress).
		WithWriteTo(cmd.ToWriteToPhysicalAddress).
		Build()

	err := p.ToPMC.Send(req)

	if err != nil {
		log.Panicf("Unable to send page migration req to PMC")
	}
	return nil
}

func (p *CommandProcessor) handlePageMigrationRsp(rsp *pagemigrationcontroller.PageMigrationRspFromPMC) error {
	now := rsp.Time()

	req := NewPageMigrationRspToDriver(now, p.ToDriver, p.Driver)

	err := p.ToDriver.Send(req)
	if err != nil {
		log.Panicf("Unable to send migration complete rsp to Driver")

	}

	return nil
}

func (p *CommandProcessor) handleFlushCommand(cmd *FlushCommand) error {
	now := cmd.Time()

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
		err := p.ToDriver.Send(p.currFlushRequest)
		if err != nil {
			panic("send failed")
		}
	}

	return nil
}

func (p *CommandProcessor) flushCache(now akita.VTimeInSec, port akita.Port) {
	flushReq := cache.FlushReqBuilder{}.
		WithSendTime(now).
		WithSrc(p.ToDispatcher).
		WithDst(port).
		Build()
	err := p.ToCaches.Send(flushReq)
	if err != nil {
		log.Panic("Fail to send flush request")
	}
	p.numCacheACK++
}

func (p *CommandProcessor) processMemCopyReq(req akita.Msg) error {
	now := req.Time()
	if req.Meta().Src == p.Driver {
		req.Meta().Dst = p.DMAEngine
		req.Meta().Src = p.ToDispatcher
		req.Meta().SendTime = now
		err := p.ToDispatcher.Send(req)
		if err != nil {
			panic(err)
		}
	} else if req.Meta().Src == p.DMAEngine {
		req.Meta().Dst = p.Driver
		req.Meta().Src = p.ToDriver
		req.Meta().SendTime = now
		err := p.ToDriver.Send(req)
		if err != nil {
			panic(err)
		}
	} else {
		log.Panic("The request sent to the command processor has unknown src")
	}
	return nil
}

// NewCommandProcessor creates a new CommandProcessor
func NewCommandProcessor(name string, engine akita.Engine) *CommandProcessor {
	c := new(CommandProcessor)
	c.ComponentBase = akita.NewComponentBase(name)

	c.engine = engine
	c.Freq = 1 * akita.GHz

	c.kernelFixedOverheadInCycles = 1600

	c.ToDriver = akita.NewLimitNumMsgPort(c, 1)
	c.ToDispatcher = akita.NewLimitNumMsgPort(c, 1)

	c.ToCUs = akita.NewLimitNumMsgPort(c, 1)
	c.ToTLBs = akita.NewLimitNumMsgPort(c, 1)
	c.ToRDMA = akita.NewLimitNumMsgPort(c, 1)
	c.ToPMC = akita.NewLimitNumMsgPort(c, 1)
	c.ToAddressTranslators = akita.NewLimitNumMsgPort(c, 1)
	c.ToCaches = akita.NewLimitNumMsgPort(c, 1)

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
