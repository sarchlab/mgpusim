package emu

import (
	"fmt"
	"reflect"

	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/core/event"
)

// CommandProcessor is a Yaotsu component that is responsible for receiving
// requests from the driver and dispatch the requests to other parts of the
// GPU.
//
//     ToDriver <=> Receive request and send feedback to the driver
//     ToDispatcher <=> Dispatcher of compute kernels
type CommandProcessor struct {
	*conn.BasicComponent

	Dispatcher conn.Component
}

// NewCommandProcessor creates a new CommandProcessor
func NewCommandProcessor(name string) *CommandProcessor {
	c := new(CommandProcessor)
	c.BasicComponent = conn.NewBasicComponent(name)

	c.AddPort("ToDriver")
	c.AddPort("ToDispatcher")

	return c
}

func (p *CommandProcessor) handleLaunchKernelReq(req *LaunchKernelReq) *conn.Error {
	req.SetSource(p)
	req.SetDestination(p.Dispatcher)
	return p.GetConnection("ToDispatcher").Send(req)
}

// Receive processes the incomming requests
func (p *CommandProcessor) Receive(req conn.Request) *conn.Error {
	switch req := req.(type) {
	case *LaunchKernelReq:
		return p.handleLaunchKernelReq(req)
	default:
		return conn.NewError(
			fmt.Sprintf("cannot process request %s", reflect.TypeOf(req)), false, 0)
	}
}

// Handle processes the events that is scheduled for the CommandProcessor
func (p *CommandProcessor) Handle(e event.Event) error {
	return nil
}
