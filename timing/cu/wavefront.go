package cu

import "gitlab.com/yaotsu/gcn3/kernels"
import "gitlab.com/yaotsu/gcn3/insts"

// WfState marks what state that wavefront it in.
type WfState int

// A list of all possible WfState
const (
	Dispatching WfState = iota // Dispatching in progress, not ready to run
	Ready                      // Allow the scheduler to schedule instruction
	Running                    // Instruction in fight
	Completed                  // Wavefront completed
)

// A Wavefront in the timing package contains the information of the progress
// of a wavefront
type Wavefront struct {
	*kernels.Wavefront

	CodeObject *insts.HsaCo
	Packet     *kernels.HsaKernelDispatchPacket

	Status WfState

	PC          uint64
	FetchBuffer []byte
	SIMDID      int
	SRegOffset  int
	VRegOffset  int
	LDSOffset   int
}

// A WorkGroup is a wrapper for the kernels.WorkGroup
type WorkGroup struct {
	*kernels.WorkGroup

	Wfs []*Wavefront
}

// NewWorkGroup returns a newly constructed WorkGroup
func NewWorkGroup(raw *kernels.WorkGroup) *WorkGroup {
	wg := new(WorkGroup)
	wg.WorkGroup = raw
	wg.Wfs = make([]*Wavefront, 0)
	return wg
}
