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
	if len(wfPools) == 0 {
		return []*Wavefront{}
	}

	a.lastSIMDID++
	if a.lastSIMDID >= len(wfPools) {
		a.lastSIMDID = 0
	}

	typeMask := make([]bool, 7)
	wfPool := wfPools[a.lastSIMDID]
	list := make([]*Wavefront, 0)
	for _, wf := range wfPool.wfs {
		wf.RLock()
		if wf.State == WfFetched && typeMask[wf.Inst.ExeUnit] == false {
			list = append(list, wf)
			typeMask[wf.Inst.ExeUnit] = true
		}
		wf.RUnlock()
	}
	return list
}
