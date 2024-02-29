package smunit

import (
	"fmt"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/samples/runner"
)

type SMUnit struct {
	sim.TickingComponent
	runner.TraceableComponent

	parentNameString string
	nameID           string

	RegisterFile runner.TraceableComponent
	ALUInt32     []runner.TraceableComponent
	ALUInt32Port []sim.Port
	ALUInt32Conn []sim.Connection
}

func (s *SMUnit) Name() string {
	return fmt.Sprintf("%s.SMUnit[%s]", s.parentNameString, s.nameID)
}

// AcceptHook registers a hook.
func (s *SMUnit) AcceptHook(hook sim.Hook) {
	panic("not implemented") // TODO: Implement
}

// NumHooks returns the number of hooks registered.
func (s *SMUnit) NumHooks() int {
	panic("not implemented") // TODO: Implement
}

// Hooks returns all the hooks registered.
func (s *SMUnit) Hooks() []sim.Hook {
	panic("not implemented") // TODO: Implement
}

func (s *SMUnit) InvokeHook(_ sim.HookCtx) {
	panic("not implemented") // TODO: Implement
}

func (s *SMUnit) Tick(now sim.VTimeInSec) bool {
	return true
}

func (s *SMUnit) Execute(warp *nvidia.Warp) {

}

func (s *SMUnit) IsFree() bool {
	return true
}
