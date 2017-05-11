package cu

// IssueDirection tells the category of an instruction
type IssueDirection int

// A list of all possible issue directions
const (
	IssueDirVALU IssueDirection = iota
	IssueDirScalar
	IssueDirVMem
	IssueDirBranch
	IssueDirLDS
	IssueDirGDS
	IssueDirInternal
)

// An IssueArbiter decides which wavefront can issue instruction
type IssueArbiter struct {
	lastSIMDID int
}

// NewIssueArbiter returns a newly created IssueArbiter
func NewIssueArbiter() *IssueArbiter {
	a := new(IssueArbiter)
	a.lastSIMDID = -1
	return a
}

// Arbitrate will take a round-robin fashion at SIMD level. For wavefronts
// in each SIMD, oldest first.
func (a *IssueArbiter) Arbitrate(wfPools []*WavefrontPool) []*Wavefront {
	list := make([]*Wavefront, 0)
	return list
}
