package cu

import "github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"

// An IssueArbiter decides which wavefront can issue instruction
type IssueArbiter struct {
	cu                *ComputeUnit
	scoreboardEnabled bool

	// rotation serves one SIMD per cycle instead of all of them.
	rotation bool
}

// NewIssueArbiter returns a newly created IssueArbiter
func NewIssueArbiter(cu *ComputeUnit) *IssueArbiter { return &IssueArbiter{cu: cu} }
func (a *IssueArbiter) state() *SchedulerState      { return &a.cu.comp.State.Scheduler }

// Arbitrate will take a round-robin fashion at SIMD level. For wavefronts
// in each SIMD, oldest first. With rotation, only the SIMD whose turn it is
// in this cycle is served.
func (a *IssueArbiter) Arbitrate(
	wfPools []*WavefrontPool,
) []*Wavefront {
	if a.isAllWfPoolsEmpty(wfPools) {
		return []*Wavefront{}
	}

	wfToIssue := make([]*Wavefront, 0)
	if a.rotation {
		return a.arbitratePool(wfPools[a.cu.servedSIMD(len(wfPools))], wfToIssue)
	}

	for i := 0; i < len(wfPools); i++ {
		simdID := (a.state().LastSIMD + i) % len(wfPools)
		wfToIssue = a.arbitratePool(wfPools[simdID], wfToIssue)
	}

	a.state().LastSIMD = (a.state().LastSIMD + 1) % len(wfPools)

	return wfToIssue
}

// arbitratePool picks, for one SIMD, at most one ready wavefront per
// execution unit type, oldest wavefront first.
func (a *IssueArbiter) arbitratePool(
	wfPool *WavefrontPool,
	wfToIssue []*Wavefront,
) []*Wavefront {
	typeMask := make([]bool, 7)
	for _, handle := range wfPool.ids() {
		wf := wfPool.cu.wave(handle)
		if wf.execution().State != wavefront.WfReady {
			continue
		}

		next := wf.NextInst()
		if next == nil || a.cu.issueHeldBack(wf) {
			continue
		}
		if a.scoreboardEnabled && wf.scoreboard() != nil {
			sb := wf.scoreboard()
			if sb.HasHazard(next) {
				continue
			}
		}

		if !typeMask[next.ExeUnit] {
			wfToIssue = append(wfToIssue, wf)
			typeMask[next.ExeUnit] = true
		}
	}

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
