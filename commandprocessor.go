package gcn3

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
)

type Resettable interface {
	Reset()
}

// CommandProcessor is a Yaotsu component that is responsible for receiving
// requests from the driver and dispatch the requests to other parts of the
// GPU.
//
//     ToDriver <=> Receive request and send feedback to the driver
//     ToDispatcher <=> Dispatcher of compute kernels
type CommandProcessor struct {
	*core.ComponentBase

	engine core.Engine
	Freq   core.Freq

	Dispatcher *core.Port
	DMAEngine  *core.Port
	Driver     *core.Port

	ToResetAfterKernel []Resettable

	ToDriver     *core.Port
	ToDispatcher *core.Port
}

func (p *CommandProcessor) NotifyRecv(now core.VTimeInSec, port *core.Port) {
	req := port.Retrieve(now)
	core.ProcessReqAsEvent(req, p.engine, p.Freq)
}

func (p *CommandProcessor) NotifyPortFree(now core.VTimeInSec, port *core.Port) {
	panic("implement me")
}

// Handle processes the events that is scheduled for the CommandProcessor
func (p *CommandProcessor) Handle(e core.Event) error {
	switch req := e.(type) {
	case *kernels.LaunchKernelReq:
		return p.processLaunchKernelReq(req)
	case *MemCopyD2HReq:
		return p.processMemCopyReq(req)
	case *MemCopyH2DReq:
		return p.processMemCopyReq(req)
	default:
		log.Panicf("cannot process request %s", reflect.TypeOf(req))
	}
	return nil
}

func (p *CommandProcessor) processLaunchKernelReq(
	req *kernels.LaunchKernelReq,
) error {
	now := req.Time()
	if req.Src() == p.Driver {
		req.SetDst(p.Dispatcher)
		req.SetSrc(p.ToDispatcher)
		req.SetSendTime(now)
		p.ToDispatcher.Send(req)
	} else if req.Src() == p.Dispatcher {
		for _, r := range p.ToResetAfterKernel {
			r.Reset()
		}
		req.SetDst(p.Driver)
		req.SetSrc(p.ToDriver)
		req.SetSendTime(now)
		p.ToDriver.Send(req)
	} else {
		log.Panic("The request sent to the command processor has unknown src")
	}
	return nil
}

func (p *CommandProcessor) processMemCopyReq(req core.Req) error {
	now := req.Time()
	if req.Src() == p.Driver {
		req.SetDst(p.DMAEngine)
		req.SetSrc(p.ToDispatcher)
		req.SetSendTime(now)
		p.ToDispatcher.Send(req)
	} else if req.Src() == p.DMAEngine {
		req.SetDst(p.Driver)
		req.SetSrc(p.ToDriver)
		req.SetSendTime(now)
		p.ToDriver.Send(req)
	} else {
		log.Panic("The request sent to the command processor has unknown src")
	}
	return nil
}

// NewCommandProcessor creates a new CommandProcessor
func NewCommandProcessor(name string, engine core.Engine) *CommandProcessor {
	c := new(CommandProcessor)
	c.ComponentBase = core.NewComponentBase(name)

	c.engine = engine
	c.Freq = 1 * core.GHz

	c.ToDriver = core.NewPort(c)
	c.ToDispatcher = core.NewPort(c)

	return c
}
