package cu

import "gitlab.com/yaotsu/core"

// A ComputeUnit in the timing package provides a detailed and accurate
// simulation of a GCN3 ComputeUnit
type ComputeUnit struct {
	*core.ComponentBase

	DataMem *core.Component

	Scheduler *Scheduler

	VRegFiles []*RegCtrl
	SRegFile  *RegCtrl
}

// NewComputeUnit returns a newly constructed compute unit
func NewComputeUnit(name string) *ComputeUnit {
	computeUnit := new(ComputeUnit)
	computeUnit.ComponentBase = core.NewComponentBase(name)

	computeUnit.VRegFiles = make([]*RegCtrl, 0)

	return computeUnit
}
