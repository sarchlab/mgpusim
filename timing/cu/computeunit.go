package cu

import "gitlab.com/yaotsu/core"

// A ComputeUnit in the timing package provides a detailed and accurate
// simulation of a GCN3 ComputeUnit
type ComputeUnit struct {
	*core.ComponentBase

	Scheduler    *Scheduler
	VMemDecode   core.Component
	ScalarDecode core.Component
	VectorDecode core.Component
	LDSDecode    core.Component

	VRegFiles []*RegCtrl
	SRegFile  *RegCtrl

	DataMem *core.Component
}

// NewComputeUnit returns a newly constructed compute unit
func NewComputeUnit(name string) *ComputeUnit {
	computeUnit := new(ComputeUnit)
	computeUnit.ComponentBase = core.NewComponentBase(name)

	computeUnit.VRegFiles = make([]*RegCtrl, 0)
	return computeUnit
}
