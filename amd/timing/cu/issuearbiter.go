package cu

import "github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"

// An IssueArbiter decides which wavefront can issue instruction
type IssueArbiter struct {
	cu                *ComputeUnit
	scoreboardEnabled bool
}

// NewIssueArbiter returns a newly created IssueArbiter
func NewIssueArbiter(cu *ComputeUnit) *IssueArbiter { return &IssueArbiter{cu: cu} }
func (a *IssueArbiter) state() *SchedulerState      { return &a.cu.comp.State.Scheduler }

// Arbitrate will take a round-robin fashion at SIMD level. For wavefronts
// in each SIMD, oldest first.
func (a *IssueArbiter) Arbitrate(
	wfPools []*WavefrontPool,
) []*Wavefront {
	if a.isAllWfPoolsEmpty(wfPools) {
		return []*Wavefront{}
	}

	wfToIssue := make([]*Wavefront, 0)
	for i := 0; i < len(wfPools); i++ {
		simdID := (a.state().LastSIMD + i) % len(wfPools)

		typeMask := make([]bool, 7)
		wfPool := wfPools[simdID]
		for _, handle := range wfPool.ids() {
			wf := wfPool.cu.wave(handle)
			if wf.execution().State != wavefront.WfReady {
				continue
			}

			next := wf.NextInst()
			if next == nil {
				continue
			}
			if a.scoreboardEnabled && wf.scoreboard() != nil {
				sb := wf.scoreboard()
				if sb.HasHazard(next) {
					continue
				}
			}

			if typeMask[next.ExeUnit] == false {
				wfToIssue = append(wfToIssue, wf)
				typeMask[next.ExeUnit] = true
			}
		}
	}

	a.state().LastSIMD = (a.state().LastSIMD + 1) % len(wfPools)

	return wfToIssue
}

func (a *IssueArbiter) moveToNextSIMD(wfPools []*WavefrontPool) {
	a.state().LastSIMD++
	if a.state().LastSIMD >= len(wfPools) {
		a.state().LastSIMD = 0
	}
}

func (a *IssueArbiter) isAllWfPoolsEmpty(wfPools []*WavefrontPool) bool {
	for _, wfPool := range wfPools {
		if len(wfPool.ids()) != 0 {
			return false
		}
	}
	return true
}
