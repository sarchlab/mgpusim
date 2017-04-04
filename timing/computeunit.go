package timing

import "gitlab.com/yaotsu/core"

// A ComputeUnit in the timing package provides a detailed and accurate
// simulation of a GCN3 ComputeUnit
type ComputeUnit struct {
	*core.BasicComponent

	DataMem *core.Component

	Scheduler *Scheduler

	VRegFiles []*RegCtrl
	SRegFiles []*RegCtrl
}
