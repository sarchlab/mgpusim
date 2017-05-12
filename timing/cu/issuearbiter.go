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
	IssueDirInternal
	issueDirCount
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
	if len(wfPools) == 0 {
		return []*Wavefront{}
	}

	a.lastSIMDID++
	if a.lastSIMDID >= len(wfPools) {
		a.lastSIMDID = 0
	}

	typeMask := make([]bool, int(issueDirCount))
	wfPool := wfPools[a.lastSIMDID]
	list := make([]*Wavefront, 0)
	for _, wf := range wfPool.wfs {
		if wf.State == WfFetched && typeMask[wf.IssueDir] == false {
			list = append(list, wf)
			typeMask[wf.IssueDir] = true
		}
	}
	return list
}
