package cu

import (
	"testing"

	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

type cuMilestoneRecorder struct {
	tracing.NopTracer

	milestones []tracing.Milestone
}

func (r *cuMilestoneRecorder) AddMilestone(m tracing.Milestone) {
	r.milestones = append(r.milestones, m)
}

func (r *cuMilestoneRecorder) has(
	taskID uint64, kind tracing.MilestoneKind, what string,
) bool {
	for _, m := range r.milestones {
		if m.TaskID == taskID && m.Kind == kind && m.What == what {
			return true
		}
	}

	return false
}

// When an S_WAITCNT's outstanding memory accesses have drained, the scheduler
// must record a "data" milestone on the instruction's task, attributing the
// time the wavefront spent waiting for memory.
func TestSWaitCntRecordsDataMilestone(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	tracing.CollectTrace(cu.comp, rec)
	s := NewScheduler(cu, nil, nil)

	wf := wavefront.NewWavefront(kernels.NewWavefront())
	inst := wavefront.NewInst(insts.NewInst())
	wf.SetDynamicInst(inst)
	// Outstanding counts (0) already satisfy the requested counts (0), so the
	// wait resolves immediately.

	madeProgress, completed := s.evalSWaitCnt(wf)

	if !madeProgress || !completed {
		t.Fatalf("expected S_WAITCNT to retire, got progress=%v completed=%v",
			madeProgress, completed)
	}
	if !rec.has(inst.ID, tracing.MilestoneKindData, "s_waitcnt") {
		t.Fatalf("expected a data milestone for s_waitcnt, got %+v",
			rec.milestones)
	}
}
