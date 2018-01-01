package timing

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/util"
)

// A ComputeUnit in the timing package provides a detailed and accurate
// simulation of a GCN3 ComputeUnit
type ComputeUnit struct {
	*core.ComponentBase

	engine core.Engine
	Freq   util.Freq

	Scheduler    core.Component
	VMemDecode   core.Component
	ScalarDecode core.Component
	VectorDecode core.Component
	LDSDecode    core.Component

	SIMDUnits  []core.Component
	LDSUnit    core.Component
	VMemUnit   core.Component
	ScalarUnit core.Component
	BranchUnit core.Component

	VRegFiles []*RegCtrl
	SRegFile  *RegCtrl

	DataMem *core.Component
}

// NewComputeUnit returns a newly constructed compute unit
func NewComputeUnit(name string) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.ComponentBase = core.NewComponentBase(name)

	cu.VRegFiles = make([]*RegCtrl, 0)
	cu.SIMDUnits = make([]core.Component, 0)

	cu.AddPort("ToACE")
	cu.AddPort("ToInstMem")
	cu.AddPort("ToDataMem")

	return cu
}

// Recv processes incoming requests
func (cu *ComputeUnit) Recv(req core.Req) *core.Error {
	util.ProcessReqAsEvent(req, cu.engine, cu.Freq)
	return nil
}

// Handle processes that events that are scheduled on the ComputeUnit
func (cu *ComputeUnit) Handle(evt core.Event) error {
	cu.InvokeHook(evt, cu, core.BeforeEvent, nil)
	defer cu.InvokeHook(evt, cu, core.AfterEvent, nil)

	switch evt := evt.(type) {
	case *core.TickEvent:
		return cu.handleTickEvent(evt)
	}

	return nil
}

func (cu *ComputeUnit) handleTickEvent(evt *core.TickEvent) error {
	return nil
}
