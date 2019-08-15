package gcn3

import (
<<<<<<< HEAD
=======
	"fmt"
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
	"log"
	"reflect"

	"gitlab.com/akita/gcn3/timing/caches/l1v"
	"gitlab.com/akita/util/tracing"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/cache/writeback"
	"gitlab.com/akita/mem/idealmemcontroller"
	"gitlab.com/akita/mem/vm"
)

<<<<<<< HEAD
// CommandProcessor is an Akita component that is responsible for receiving
=======
// CommandProcessor is a Akita component that is responsible for receiving
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
// requests from the driver and dispatch the requests to other parts of the
// GPU.
//
//     ToDriver <=> Receive request and send feedback to the driver
//     ToDispatcher <=> Dispatcher of compute kernels
type CommandProcessor struct {
	*akita.ComponentBase

	engine akita.Engine
	Freq   akita.Freq

	Dispatcher akita.Port
	DMAEngine  akita.Port
	Driver     akita.Port
	VMModules  []akita.Port
	CUs        []akita.Port

	ToDriver     akita.Port
	ToDispatcher akita.Port

	L1VCaches       []*l1v.Cache
	L1SCaches       []*l1v.Cache
	L1ICaches       []*l1v.Cache
	L2Caches        []*writeback.Cache
	DRAMControllers []*idealmemcontroller.Comp
	ToCUs           akita.Port
	ToVMModules     akita.Port

	kernelFixedOverheadInCycles int

	curShootdownRequest *ShootDownCommand
	currFlushRequest    *FlushCommand

	numCUs     uint64
	numVMUnits uint64

	numCURecvdAck uint64
	numVMRecvdAck uint64
	numFlushACK   uint64

	shootDownInProcess bool
<<<<<<< HEAD

	bottomReqIDToTopReqMap map[string]*LaunchKernelReq
=======
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
}

func (p *CommandProcessor) NotifyRecv(
	now akita.VTimeInSec,
	port akita.Port,
) {
	req := port.Retrieve(now)
	akita.ProcessReqAsEvent(req, p.engine, p.Freq)
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
	case *mem.InvalidDoneRsp:
		// Do nothing
	case *ShootDownCommand:
		return p.handleShootdownCommand(req)
	case *CUPipelineDrainRsp:
		return p.handleCUPipelineDrainRsp(req)
	case *vm.InvalidationCompleteRsp:
		return p.handleVMInvalidationRsp(req)

	default:
		log.Panicf("cannot process request %s", reflect.TypeOf(req))
	}
	return nil
}

func (p *CommandProcessor) processLaunchKernelReq(
	req *LaunchKernelReq,
) error {
	now := req.Time()
	if req.Src() == p.Driver {
<<<<<<< HEAD
		reqToBottom := NewLaunchKernelReq(now, p.ToDispatcher, p.Dispatcher)
		reqToBottom.PID = req.PID
		reqToBottom.Packet = req.Packet
		reqToBottom.PacketAddress = req.PacketAddress
		reqToBottom.HsaCo = req.HsaCo
		reqToBottom.SetSendTime(now)
		p.ToDispatcher.Send(reqToBottom)
		p.bottomReqIDToTopReqMap[reqToBottom.ID] = req
		tracing.TraceReqReceive(req, now, p)
		tracing.TraceReqInitiate(reqToBottom, now, p, tracing.ReqIDAtReceiver(req, p))
	} else if req.Src() == p.Dispatcher {
=======
		req.SetDst(p.Dispatcher)
		req.SetSrc(p.ToDispatcher)
		req.SetSendTime(now)
		p.ToDispatcher.Send(req)
		tracing.StartTask(
			fmt.Sprintf("%s@%s", req.GetID(), p.Name()),
			req.GetID(),
			req.Time(),
			p,
			"Req", "Launch Kernel",
			nil,
		)
	} else if req.Src() == p.Dispatcher {
		req.SetDst(p.Driver)
		req.SetSrc(p.ToDriver)
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
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
<<<<<<< HEAD

	req := evt.Req
	originalReq := p.bottomReqIDToTopReqMap[req.ID]
	originalReq.SetSendTime(now)
	originalReq.SwapSrcAndDst()
	p.ToDriver.Send(originalReq)
	tracing.TraceReqFinalize(req, now, p)
	tracing.TraceReqComplete(originalReq, now, p)
=======
	evt.Req.SetSendTime(now)
	p.ToDriver.Send(evt.Req)
	tracing.EndTask(
		fmt.Sprintf("%s@%s", evt.Req.GetID(), p.Name()),
		now, p)
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
	return nil
}

func (p *CommandProcessor) handleShootdownCommand(cmd *ShootDownCommand) error {
	now := cmd.Time()

	if p.shootDownInProcess == true {
		return nil
	}

	p.curShootdownRequest = cmd
	p.shootDownInProcess = true

	for i := 0; i < len(p.CUs); i++ {
		req := NewCUPipelineDrainReq(now, p.ToCUs, p.CUs[i])
		err := p.ToCUs.Send(req)
		if err != nil {
			log.Panicf("failed to send pipeline drain request to CU")
		}

	}

	//TODO: NotifyRecvl already does a retrieve from port. Why do it again

	return nil

}

func (p *CommandProcessor) handleCUPipelineDrainRsp(
	cmd *CUPipelineDrainRsp,
) error {
	now := cmd.Time()

	if cmd.drainPipelineComplete == true {
		p.numCURecvdAck = p.numCURecvdAck + 1

	}

	if p.numCURecvdAck == p.numCUs {
		shootDownCmd := p.curShootdownRequest
		for i := 0; i < len(p.VMModules); i++ {
			req := vm.NewPTEInvalidationReq(now, p.ToVMModules, p.VMModules[i], shootDownCmd.PID, shootDownCmd.VAddr)
			err := p.ToVMModules.Send(req)
			if err != nil {
				log.Panicf("failed to send shootdown request to VM Modules")
			}
		}
	}

	return nil
}

func (p *CommandProcessor) handleVMInvalidationRsp(
	cmd *vm.InvalidationCompleteRsp,
) error {
	now := cmd.Time()

	if cmd.InvalidationDone == true {
		p.numVMRecvdAck = p.numVMRecvdAck + 1
	}

	if p.numVMRecvdAck == p.numVMUnits {
		req := NewShootdownCompleteRsp(now, p.ToDriver, p.Driver)
		req.shootDownComplete = true
		err := p.ToDriver.Send(req)
		if err != nil {
			log.Panicf("Failed to send shootdown complete ack to driver")
		}

		p.shootDownInProcess = false
		p.numVMRecvdAck = 0
		p.numCURecvdAck = 0
	}

	return nil
}

func (p *CommandProcessor) handleFlushCommand(cmd *FlushCommand) error {
	now := cmd.Time()

	for _, c := range p.L1ICaches {
		p.flushCache(now, c.ControlPort)
	}

	for _, c := range p.L1SCaches {
		p.flushCache(now, c.ControlPort)
	}

	for _, c := range p.L1VCaches {
		p.flushCache(now, c.ControlPort)
	}

	for _, c := range p.L2Caches {
		p.flushCache(now, c.ControlPort)
	}

	p.currFlushRequest = cmd
	if p.numFlushACK == 0 {
		p.currFlushRequest.SwapSrcAndDst()
		p.currFlushRequest.SetSendTime(now)
		err := p.ToDriver.Send(p.currFlushRequest)
		if err != nil {
			panic("send failed")
		}
	}

	return nil
}

func (p *CommandProcessor) flushCache(now akita.VTimeInSec, port akita.Port) {
	flushReq := cache.NewFlushReq(now, p.ToDispatcher, port)
	err := p.ToDispatcher.Send(flushReq)
	if err != nil {
		log.Panic("Fail to send flush request")
	}
	p.numFlushACK++
}

func (p *CommandProcessor) handleCacheFlushRsp(
	req *cache.FlushRsp,
) error {
	p.numFlushACK--
	if p.numFlushACK == 0 {
		p.currFlushRequest.SwapSrcAndDst()
		p.currFlushRequest.SetSendTime(req.Time())
		err := p.ToDriver.Send(p.currFlushRequest)
		if err != nil {
			panic("send failed")
		}
	}

	return nil
}

func (p *CommandProcessor) processMemCopyReq(req akita.Req) error {
	now := req.Time()
	if req.Src() == p.Driver {
		req.SetDst(p.DMAEngine)
		req.SetSrc(p.ToDispatcher)
		req.SetSendTime(now)
		err := p.ToDispatcher.Send(req)
		if err != nil {
			panic(err)
		}

	} else if req.Src() == p.DMAEngine {
		req.SetDst(p.Driver)
		req.SetSrc(p.ToDriver)
		req.SetSendTime(now)
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
	c.L2Caches = make([]*writeback.Cache, 0)

	c.ToDriver = akita.NewLimitNumReqPort(c, 1)
	c.ToDispatcher = akita.NewLimitNumReqPort(c, 1)

	c.ToCUs = akita.NewLimitNumReqPort(c, 1)
	c.ToVMModules = akita.NewLimitNumReqPort(c, 1)

<<<<<<< HEAD
	c.bottomReqIDToTopReqMap = make(map[string]*LaunchKernelReq)

=======
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
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
