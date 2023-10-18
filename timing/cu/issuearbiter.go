package cu

import "github.com/sarchlab/mgpusim/v3/timing/wavefront"

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
func (a *IssueArbiter) Arbitrate(
	wfPools []*WavefrontPool,
) []*wavefront.Wavefront {
	if a.isAllWfPoolsEmpty(wfPools) {
		return []*wavefront.Wavefront{}
	}

    originalSIMDID := a.lastSIMDID

	list := make([]*wavefront.Wavefront, 0)
    for len(list) == 0  {
	    a.moveToNextSIMD(wfPools)
	    for len(wfPools[a.lastSIMDID].wfs) == 0 {
	        a.moveToNextSIMD(wfPools)
            if a.lastSIMDID == originalSIMDID {
                break
            }
	    }
        if len(wfPools[a.lastSIMDID].wfs) != 0 {

	        typeMask := make([]bool, 7)
	        wfPool := wfPools[a.lastSIMDID]
	        for _, wf := range wfPool.wfs {
                if wf.State != wavefront.WfReady || wf.InstToIssue == nil {
                    continue
                }

                if typeMask[wf.InstToIssue.ExeUnit] == false {
                    list = append(list, wf)
                    typeMask[wf.InstToIssue.ExeUnit] = true
                }
	        }
        }
        if a.lastSIMDID == originalSIMDID {
            break
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
		if len(wfPool.wfs) != 0 {
			return false
		}
	}
	return true
}
