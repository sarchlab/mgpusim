package cu

import "github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"

// An IssueArbiter decides which wavefront can issue instruction
type IssueArbiter struct {
	state             *SchedulerState
	scoreboardEnabled bool
}

// NewIssueArbiter returns a newly created IssueArbiter
func NewIssueArbiter() *IssueArbiter {
	a := &IssueArbiter{state: &SchedulerState{}}
	a.state.LastSIMD = 0
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
		simdID := (a.state.LastSIMD + i) % len(wfPools)

		typeMask := make([]bool, 7)
		wfPool := wfPools[simdID]
		for _, wf := range wfPool.waves() {
			if wf.State != wavefront.WfReady || wf.NextInst() == nil {
				continue
			}

			if a.scoreboardEnabled && wf.ScoreboardData != nil {
				sb := wf.ScoreboardData.(*Scoreboard)
				if sb.HasHazard(wf.NextInst().Inst) {
					continue
				}
			}

			if typeMask[wf.NextInst().ExeUnit] == false {
				wfToIssue = append(wfToIssue, wf)
				typeMask[wf.NextInst().ExeUnit] = true
			}
		}
	}

	a.state.LastSIMD = (a.state.LastSIMD + 1) % len(wfPools)

	return wfToIssue
}

func (a *IssueArbiter) moveToNextSIMD(wfPools []*WavefrontPool) {
	a.state.LastSIMD++
	if a.state.LastSIMD >= len(wfPools) {
		a.state.LastSIMD = 0
	}
}

func (a *IssueArbiter) isAllWfPoolsEmpty(wfPools []*WavefrontPool) bool {
	for _, wfPool := range wfPools {
		if len(wfPool.waves()) != 0 {
			return false
		}
	}
	return true
}
