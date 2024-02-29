package alu

import (
	"fmt"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type int32ALU struct {
	sim.TickingComponent

	ToSM sim.Port

	parentNameString string
	nameID           string
}

func (a *int32ALU) Name() string {
	return fmt.Sprintf("%s.INT32[%s]", a.parentNameString, a.nameID)
}

// AcceptHook registers a hook.
func (a *int32ALU) AcceptHook(hook sim.Hook) {
	panic("not implemented") // TODO: Implement
}

// NumHooks returns the number of hooks registered.
func (a *int32ALU) NumHooks() int {
	panic("not implemented") // TODO: Implement
}

// Hooks returns all the hooks registered.
func (a *int32ALU) Hooks() []sim.Hook {
	panic("not implemented") // TODO: Implement
}

func (a *int32ALU) InvokeHook(_ sim.HookCtx) {
	panic("not implemented") // TODO: Implement
}

func (a *int32ALU) Tick(now sim.VTimeInSec) bool {
	return true
}

func (a *int32ALU) Execute(inst nvidia.Instruction) {
	// do the instruction
}
