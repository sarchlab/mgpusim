package gpu

import (
	"fmt"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/samples/runner"
)

type GPU struct {
	sim.TickingComponent
	runner.TraceableComponent

	parentNameString string
	nameID           string

	dispatcher GPUDispatcher

	GPCs []runner.TraceableComponent
}

func (g *GPU) Name() string {
	return fmt.Sprintf("%s.GPU[%s]", g.parentNameString, g.nameID)
}

// AcceptHook registers a hook.
func (g *GPU) AcceptHook(hook sim.Hook) {
	panic("not implemented") // TODO: Implement
}

// NumHooks returns the number of hooks registered.
func (g *GPU) NumHooks() int {
	panic("not implemented") // TODO: Implement
}

// Hooks returns all the hooks registered.
func (g *GPU) Hooks() []sim.Hook {
	panic("not implemented") // TODO: Implement
}

func (g *GPU) InvokeHook(_ sim.HookCtx) {
	panic("not implemented") // TODO: Implement
}

func (g *GPU) Tick(now sim.VTimeInSec) bool {
	return true
}

// RunThreadBlock runs a threadblock on the GPU
// [todo] how to handle the relationship between trace.threadblock and truethreadblock
func (g *GPU) RunThreadBlock(tb *nvidia.ThreadBlock) {
	g.dispatcher.Dispatch(g, tb)
}
