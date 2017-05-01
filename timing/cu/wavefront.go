package cu

import "gitlab.com/yaotsu/gcn3/kernels"

// WfState marks what state that wavefront it in.
type WfState int

// A list of all possible WfState
const (
	Dispatching WfState = iota // Dispatching in progress, not ready to run
	Running                    // Allow the scheduler to schedule instruction
)

// A Wavefront in the timing package contains the information of the progress
// of a wavefront
type Wavefront struct {
	*kernels.Wavefront

	Status WfState

	PC          uint64
	FetchBuffer []byte
	SIMDID      int
	SRegOffset  int
	VRegOffset  int
	LDSOffset   int
}
