package timing

import "gitlab.com/yaotsu/core"

// A ComputeUnit in the timing package provides a detailed and accurate
// simulation of a GCN3 ComputeUnit
type ComputeUnit struct {
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
	computeUnit := new(ComputeUnit)

	computeUnit.VRegFiles = make([]*RegCtrl, 0)
	computeUnit.SIMDUnits = make([]core.Component, 0)
	return computeUnit
}
