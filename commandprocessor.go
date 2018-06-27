package gcn3

import (
	"fmt"
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

	Dispatcher core.Component
	DMAEngine  core.Component
	Driver     core.Component

	ToResetAfterKernel []Resettable
}

// NewCommandProcessor creates a new CommandProcessor
func NewCommandProcessor(name string) *CommandProcessor {
	c := new(CommandProcessor)
	c.ComponentBase = core.NewComponentBase(name)

	c.AddPort("ToDriver")
	c.AddPort("ToDispatcher")

	return c
}

// Recv processes the incoming requests
func (p *CommandProcessor) Recv(req core.Req) *core.Error {
	switch req := req.(type) {
	case *kernels.LaunchKernelReq:
		return p.processLaunchKernelReq(req)
	case *MemCopyD2HReq:
		return p.processMemCopyReq(req)
	case *MemCopyH2DReq:
		return p.processMemCopyReq(req)
	default:
		return core.NewError(
			fmt.Sprintf("cannot process request %s", reflect.TypeOf(req)), false, 0)
	}
}

func (p *CommandProcessor) processLaunchKernelReq(
	req *kernels.LaunchKernelReq,
) *core.Error {
	if req.Src() == p.Driver {
		req.SetDst(p.Dispatcher)
		req.SetSrc(p)
	} else if req.Src() == p.Dispatcher {
		req.SetDst(p.Driver)
		req.SetSrc(p)
		for _, r := range p.ToResetAfterKernel {
			r.Reset()
		}
	} else {
		log.Fatal("The request sent to the command processor has unknown src")
	}
	return p.GetConnection("ToDispatcher").Send(req)
}

func (p *CommandProcessor) processMemCopyReq(req core.Req) *core.Error {
	if req.Src() == p.Driver {
		req.SetDst(p.DMAEngine)
		req.SetSrc(p)
	} else if req.Src() == p.DMAEngine {
		req.SetDst(p.Driver)
		req.SetSrc(p)
	} else {
		log.Fatal("The request sent to the command processor has unknown src")
	}
	return p.GetConnection("ToDispatcher").Send(req)
}

// Handle processes the events that is scheduled for the CommandProcessor
func (p *CommandProcessor) Handle(e core.Event) error {
	return nil
}
