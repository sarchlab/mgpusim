package smunit

import (
	"fmt"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/samples/runner"
)

type RegisterFile struct {
	sim.TickingComponent
	runner.TraceableComponent

	parentNameString string

	size            int32
	rfLaneSize      int32
	buf             []byte
	byteSizePerLane int32
}

func (r *RegisterFile) Name() string {
	return fmt.Sprintf("%s.RegisterFile", r.parentNameString)
}

// AcceptHook registers a hook.
func (r *RegisterFile) AcceptHook(hook sim.Hook) {
	panic("not implemented") // TODO: Implement
}

// NumHooks returns the number of hooks registered.
func (r *RegisterFile) NumHooks() int {
	panic("not implemented") // TODO: Implement
}

// Hooks returns all the hooks registered.
func (r *RegisterFile) Hooks() []sim.Hook {
	panic("not implemented") // TODO: Implement
}

func (r *RegisterFile) InvokeHook(_ sim.HookCtx) {
	panic("not implemented") // TODO: Implement
}

func (r *RegisterFile) Tick(now sim.VTimeInSec) bool {
	return true
}

func (r *RegisterFile) Read(offset int32, width int32) {
}

func (r *RegisterFile) Write(offset int32, width int32) {
}
