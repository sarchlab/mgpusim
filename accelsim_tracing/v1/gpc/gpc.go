package gpc

import (
	"fmt"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/sm"
	"github.com/sarchlab/mgpusim/v3/samples/runner"
)

type GPC struct {
	sim.TickingComponent
	runner.TraceableComponent

	parentNameString string
	nameID           string

	SMs []runner.TraceableComponent
}

func (g *GPC) Name() string {
	return fmt.Sprintf("%s.GPC[%s]", g.parentNameString, g.nameID)
}

// AcceptHook registers a hook.
func (g *GPC) AcceptHook(hook sim.Hook) {
	panic("not implemented") // TODO: Implement
}

// NumHooks returns the number of hooks registered.
func (g *GPC) NumHooks() int {
	panic("not implemented") // TODO: Implement
}

// Hooks returns all the hooks registered.
func (g *GPC) Hooks() []sim.Hook {
	panic("not implemented") // TODO: Implement
}

func (g *GPC) InvokeHook(_ sim.HookCtx) {
	panic("not implemented") // TODO: Implement
}

func (g *GPC) IsFree() bool {
	return true
}

func (g *GPC) Tick(now sim.VTimeInSec) bool {
	return true
}

func (g *GPC) Execute(tb *nvidia.ThreadBlock) {
	for _, i := range g.SMs {
		i.(*sm.SM).Execute(tb)
	}
}
