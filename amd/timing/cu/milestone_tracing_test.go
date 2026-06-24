package cu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
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

// When a vector-memory load's last response returns, the CU must record a
// "data" milestone on the instruction's task so its lifetime is fully
// attributed — without it the bar shows an unexplained gap between the
// in-flight admission and the task end (the memory round trip).
func TestVectorMemDataReturnRecordsDataMilestone(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	tracing.CollectTrace(cu.comp, rec)

	wf := wavefront.NewWavefront(kernels.NewWavefront())
	inst := wavefront.NewInst(insts.NewInst())
	inst.ExeUnit = insts.ExeUnitVMem
	wf.SetDynamicInst(inst)

	read := memprotocol.ReadReq{
		MsgMeta: messaging.MsgMeta{ID: timing.GetIDGenerator().Generate()},
	}
	cu.InFlightVectorMemAccess = append(cu.InFlightVectorMemAccess,
		VectorMemAccessInfo{Read: &read, Inst: inst, Wavefront: wf})

	cu.handleVectorDataLoadReturn(memprotocol.DataReadyRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			RspTo: read.ID,
		},
	})

	if !rec.has(inst.ID, tracing.MilestoneKindData, "vmem") {
		t.Fatalf("expected a data milestone for the vmem return, got %+v",
			rec.milestones)
	}
}

// Sending the first vector-memory transaction closes the issue subtask and
// records a "work" milestone (not a hardware_resource one): the coalescing /
// transaction-issue phase is the unit doing work, and it is backed by the
// pipeline subtask opened at admission. The data wait is attributed only from
// this point, once a request is actually outstanding.
func TestVectorMemSendRecordsCoalesceWorkMilestone(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	tracing.CollectTrace(cu.comp, rec)

	cu.ToVectorMem = newFakePort("CU.ToVectorMem")
	vmu := NewVectorMemoryUnit(cu, nil)
	vmu.postTransactionPipelineBuffer =
		queueing.NewBuffer[VectorMemAccessInfo]("CU.PostTransBuf", 8)

	inst := wavefront.NewInst(nil)
	vmu.startIssueSubtask(inst) // as the in-flight admission would

	read := &memprotocol.ReadReq{
		MsgMeta: messaging.MsgMeta{
			ID:  timing.GetIDGenerator().Generate(),
			Src: cu.ToVectorMem.AsRemote(),
			Dst: "VectorMem",
		},
	}
	vmu.numTransactionInFlight = 1
	vmu.postTransactionPipelineBuffer.PushTyped(
		VectorMemAccessInfo{Read: read, Inst: inst})

	vmu.sendRequest()

	if !rec.has(inst.ID, tracing.MilestoneKindWork, "coalesce") {
		t.Fatalf("expected a work milestone at transaction send, got %+v",
			rec.milestones)
	}
	if _, ok := vmu.issueTaskIDs[inst.ID]; ok {
		t.Fatal("the issue subtask should be closed after the first send")
	}
}
