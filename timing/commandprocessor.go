package timing

import (
	"fmt"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
)

// CommandProcessor is a Yaotsu component that is responsible for receiving
// requests from the driver and dispatch the requests to other parts of the
// GPU.
//
//     ToDriver <=> Receive request and send feedback to the driver
//     ToDispatcher <=> Dispatcher of compute kernels
type CommandProcessor struct {
	*core.BasicComponent

	Dispatcher core.Component
}

// NewCommandProcessor creates a new CommandProcessor
func NewCommandProcessor(name string) *CommandProcessor {
	c := new(CommandProcessor)
	c.BasicComponent = core.NewBasicComponent(name)

	c.AddPort("ToDriver")
	c.AddPort("ToDispatcher")

	return c
}

func (p *CommandProcessor) processLaunchKernelReq(
	req *kernels.LaunchKernelReq,
) *core.Error {
	req.SetSrc(p)
	req.SetDst(p.Dispatcher)
	return p.GetConnection("ToDispatcher").Send(req)
}

// Recv processes the incomming requests
func (p *CommandProcessor) Recv(req core.Req) *core.Error {
	switch req := req.(type) {
	case *kernels.LaunchKernelReq:
		return p.processLaunchKernelReq(req)
	default:
		return core.NewError(
			fmt.Sprintf("cannot process request %s", reflect.TypeOf(req)), false, 0)
	}
}

// Handle processes the events that is scheduled for the CommandProcessor
func (p *CommandProcessor) Handle(e core.Event) error {
	return nil
}
