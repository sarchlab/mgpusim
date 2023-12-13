package cu

import "github.com/sarchlab/mgpusim/v3/timing/wavefront"

// An IssueArbiter decides which wavefront can issue instruction
type IssueArbiter struct {
	lastSIMDID int
}

// NewIssueArbiter returns a newly created IssueArbiter
func NewIssueArbiter() *IssueArbiter {
	a := new(IssueArbiter)
	a.lastSIMDID = 0
	return a
}

// Arbitrate will take a round-robin fashion at SIMD level. For wavefronts
// in each SIMD, oldest first.
func (a *IssueArbiter) Arbitrate(
	wfPools []*WavefrontPool,
) []*wavefront.Wavefront {
	if a.isAllWfPoolsEmpty(wfPools) {
		return []*wavefront.Wavefront{}
	}

	wfToIssue := make([]*wavefront.Wavefront, 0)
	for i := 0; i < len(wfPools); i++ {
		simdID := (a.lastSIMDID + i) % len(wfPools)

		typeMask := make([]bool, 7)
		wfPool := wfPools[simdID]
		for _, wf := range wfPool.wfs {
			if wf.State != wavefront.WfReady || wf.InstToIssue == nil {
				continue
			}

			if typeMask[wf.InstToIssue.ExeUnit] == false {
				wfToIssue = append(wfToIssue, wf)
				typeMask[wf.InstToIssue.ExeUnit] = true
			}
		}

		if len(wfToIssue) != 0 {
			a.lastSIMDID = simdID
			break
		}
	}

	// for len(wfToIssue) == 0 {
	// 	a.moveToNextSIMD(wfPools)
	// 	for len(wfPools[a.lastSIMDID].wfs) == 0 {
	// 		if a.lastSIMDID == originalSIMDID {
	// 			break
	// 		}
	// 		a.moveToNextSIMD(wfPools)
	// 	}

	// 	typeMask := make([]bool, 7)
	// 	wfPool := wfPools[a.lastSIMDID]
	// 	for _, wf := range wfPool.wfs {
	// 		if wf.State != wavefront.WfReady || wf.InstToIssue == nil {
	// 			continue
	// 		}

	// 		if typeMask[wf.InstToIssue.ExeUnit] == false {
	// 			wfToIssue = append(wfToIssue, wf)
	// 			typeMask[wf.InstToIssue.ExeUnit] = true
	// 		}
	// 	}

	// 	if a.lastSIMDID == originalSIMDID {
	// 		break
	// 	}
	// }

	return wfToIssue
}

func (a *IssueArbiter) moveToNextSIMD(wfPools []*WavefrontPool) {
	a.lastSIMDID++
	if a.lastSIMDID >= len(wfPools) {
		a.lastSIMDID = 0
	}
}

func (a *IssueArbiter) isAllWfPoolsEmpty(wfPools []*WavefrontPool) bool {
	for _, wfPool := range wfPools {
		if len(wfPool.wfs) != 0 {
			return false
		}
	}
	return true
}
