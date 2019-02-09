package gcn3

import (
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/vm"
)

type Resettable interface {
	Reset()
}

// CommandProcessor is a Akita component that is responsible for receiving
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

	CachesToReset   []akita.Port
	L2Caches        []*cache.WriteBackCache
	DRAMControllers []*mem.IdealMemController
	ToCU            akita.Port
	ToVMModules     akita.Port

	kernelFixedOverheadInCycles int

	curShootdownRequest *ShootDownCommand

	numCUs     uint64
	numVMUnits uint64

	numCURecvdAck uint64
	numVMRecvdAck uint64

	shootDownInProcess bool
}

func (p *CommandProcessor) NotifyRecv(now akita.VTimeInSec, port akita.Port) {
	req := port.Retrieve(now)
	akita.ProcessReqAsEvent(req, p.engine, p.Freq)
}

func (p *CommandProcessor) NotifyPortFree(now akita.VTimeInSec, port akita.Port) {
	//panic("implement me")
}

// Handle processes the events that is scheduled for the CommandProcessor
func (p *CommandProcessor) Handle(e akita.Event) error {
	switch req := e.(type) {
	case *LaunchKernelReq:
		return p.processLaunchKernelReq(req)
	case *ReplyKernelCompletionEvent:
		return p.handleReplyKernelCompletionEvent(req)
	case *FlushCommand:
		return p.handleFlushCommand(req)
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
		req.SetDst(p.Dispatcher)
		req.SetSrc(p.ToDispatcher)
		req.SetSendTime(now)
		p.ToDispatcher.Send(req)
	} else if req.Src() == p.Dispatcher {
		req.SetDst(p.Driver)
		req.SetSrc(p.ToDriver)
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
	evt.Req.SetSendTime(now)
	p.ToDriver.Send(evt.Req)
	return nil
}

func (p *CommandProcessor) handleFlushCommand(cmd *FlushCommand) error {
	now := cmd.Time()
	for _, c := range p.CachesToReset {
		req := mem.NewInvalidReq(now, p.ToDispatcher, c)
		err := p.ToDispatcher.Send(req)
		if err != nil {
			log.Panic("failed to send invalidation request")
		}
	}

	for i, l2Cache := range p.L2Caches {
		p.flushL2(l2Cache, p.DRAMControllers[i])
	}

	cmd.SwapSrcAndDst()
	err := p.ToDriver.Send(cmd)
	if err != nil {
		panic(err)
	}

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
		req := NewCUPipelineDrainReq(now, p.ToCU, p.CUs[i])
		err := p.ToCU.Send(req)
		if err != nil {
			log.Panicf("failed to send pipeline drain request to CU")
		}

	}

	//TODO: NotifyRecvl already does a retrieve from port. Why do it again

	return nil

}

func (p *CommandProcessor) handleCUPipelineDrainRsp(cmd *CUPipelineDrainRsp) error {
	now := cmd.Time()

	if cmd.drainPipelineComplete == true {
		p.numCURecvdAck = p.numCURecvdAck + 1

	}

	if p.numCURecvdAck == p.numCUs {
		shootDownCmd := p.curShootdownRequest
		for i := 0; i < len(p.VMModules); i++ {
			req := vm.NewPTEInvalidationReq(now, p.ToVMModules, p.VMModules[i], shootDownCmd.pID, shootDownCmd.vAddr)
			err := p.ToVMModules.Send(req)
			if err != nil {
				log.Panicf("failed to send shootdown request to VM Modules")
			}
		}

	}

	return nil
}

func (p *CommandProcessor) handleVMInvalidationRsp(cmd *vm.InvalidationCompleteRsp) error {
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

func (p *CommandProcessor) flushL2(l2 *cache.WriteBackCache, dram *mem.IdealMemController) {
	// FIXME: This is magic, remove
	dir := l2.Directory.(*cache.DirectoryImpl)
	for _, set := range dir.Sets {
		for _, block := range set.Blocks {
			if block.IsLocked {
				log.Printf("block locked 0x%x.", block.Tag)
			}

			if block.IsDirty && block.IsValid {
				cacheData, _ := l2.Storage.Read(block.CacheAddress, uint64(dir.BlockSize))
				addr := block.Tag
				if dram.AddressConverter != nil {
					addr = dram.AddressConverter.ConvertExternalToInternal(addr)
				}
				err := dram.Storage.Write(addr, cacheData)
				if err != nil {
					panic(err)
				}
			}
			block.IsValid = false
			block.IsDirty = false
		}
	}
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
	c.L2Caches = make([]*cache.WriteBackCache, 0)

	c.ToDriver = akita.NewLimitNumReqPort(c, 1)
	c.ToDispatcher = akita.NewLimitNumReqPort(c, 1)

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
