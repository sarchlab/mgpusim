package timing

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
	if a.isAllWfPoolsEmpty(wfPools) {
		return []*Wavefront{}
	}

	a.moveToNextSIMD(wfPools)
	for len(wfPools[a.lastSIMDID].wfs) == 0 {
		a.moveToNextSIMD(wfPools)
	}

	typeMask := make([]bool, 7)
	wfPool := wfPools[a.lastSIMDID]
	list := make([]*Wavefront, 0)
	for _, wf := range wfPool.wfs {
		if wf.State != WfReady || wf.InstToIssue == nil {
			continue
		}

		if typeMask[wf.InstToIssue.ExeUnit] == false {
			list = append(list, wf)
			typeMask[wf.InstToIssue.ExeUnit] = true
		}
	}
	return list
}

func (a *IssueArbiter) moveToNextSIMD(wfPools []*WavefrontPool) {
	a.lastSIMDID++
	if a.lastSIMDID >= len(wfPools) {
		a.lastSIMDID = 0
	}
}

func (a *IssueArbiter) isAllWfPoolsEmpty(wfPools []*WavefrontPool) bool {
	for _, wfPool := range wfPools {
		if len(wfPool.wfs) == 0 {
			return false
		}
	}
	return true
}
