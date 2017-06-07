package emu

import "gitlab.com/yaotsu/core"

// A ComputeUnit in the emu package is a component that omit the pipeline design
// but can still run the GCN3 instructions.
//
//     ToDispatcher <=> The port that connect the CU with the dispatcher
//
type ComputeUnit struct {
	*core.ComponentBase

	engine core.Engine
	Freq   core.Freq
}

// NewComputeUnit creates a new ComputeUnit with the given name
func NewComputeUnit(name string, engine core.Engine) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.ComponentBase = core.NewComponentBase(name)

	cu.engine = engine

	return cu
}

// Recv accepts requests from other components
func (cu *ComputeUnit) Recv(req core.Req) *core.Error {
	return nil
}

// Handle defines the behavior on event scheduled on the ComputeUnit
func (cu *ComputeUnit) Handle(evt core.Event) error {
	return nil
}
