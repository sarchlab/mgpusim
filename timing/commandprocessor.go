package timing

import (
	"gitlab.com/yaotsu/core"
)

// A CommandProcessor serves as the gateway of a GPU. All the request goes into
// a GPU first arrives at the CommandProcessor and then dispatched to the
// function units.
type CommandProcessor struct {
	*core.BasicComponent
}

// NewCommandProcessor returns a newly created command processor
func NewCommandProcessor(name string) *CommandProcessor {
	cp := new(CommandProcessor)
	cp.BasicComponent = core.NewBasicComponent(name)
	return cp
}
