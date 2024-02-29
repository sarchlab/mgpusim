package sm

import (
	"fmt"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/samples/runner"
)

type SM struct {
	sim.TickingComponent
	runner.TraceableComponent

	parentNameString string
	nameID           string

	SMUnits    []runner.TraceableComponent
	dispatcher SMDispatcher
}

func (s *SM) Name() string {
	return fmt.Sprintf("%s.SM[%s]", s.parentNameString, s.nameID)
}

// AcceptHook registers a hook.
func (s *SM) AcceptHook(hook sim.Hook) {
	panic("not implemented") // TODO: Implement
}

// NumHooks returns the number of hooks registered.
func (s *SM) NumHooks() int {
	panic("not implemented") // TODO: Implement
}

// Hooks returns all the hooks registered.
func (s *SM) Hooks() []sim.Hook {
	panic("not implemented") // TODO: Implement
}

func (s *SM) InvokeHook(_ sim.HookCtx) {
	panic("not implemented") // TODO: Implement
}

func (s *SM) Tick(now sim.VTimeInSec) bool {
	return true
}

func (s *SM) Execute(tb *nvidia.ThreadBlock) {
	s.dispatcher.Dispatch(s, tb)
}
