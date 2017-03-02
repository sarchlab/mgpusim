package emulator

import (
	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/core/event"
)

// CommandProcessor is responsible for receiving requests from the driver and
// dispatch the requests to other parts of the GPU.
//
// CommandProcessor is a Yaotsu Component. It comnunicates with the driver
// through the "ToDriver" port. It communicates with the Dispatcher with the
// "ToDispatcher" port. It also communicates with the GPU memory via the
// "ToDram" port, and commhnicates with the L2Cache via the "ToL2Cache" port.
type CommandProcessor struct {
	*conn.BasicComponent
}

// NewCommandProcessor creates a new CommandProcessor
func NewCommandProcessor(name string) *CommandProcessor {
	c := &CommandProcessor{conn.NewBasicComponent(name)}
	c.AddPort("ToDriver")
	return c
}

// Receive processes the incomming requests
func (p *CommandProcessor) Receive(req conn.Request) *conn.Error {
	return nil
}

// Handle processes the events that is scheduled for the CommandProcessor
func (p *CommandProcessor) Handle(e event.Event) error {
	return nil
}
