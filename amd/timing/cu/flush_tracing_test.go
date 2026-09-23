package cu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/akita/v5/tracing/tracingtest"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// A pipeline flush drops the instructions sitting in the sub-unit pipeline
// stages and the scheduler's internalExecuting slice; those wavefronts are in
// WfRunning and re-issue fresh after the restart, so their "inst" tasks must be
// ended on flush or they leak.
func TestEndInflightTracingTasksEndsRunningInstTasks(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(cu.comp, rec)

	pool := NewWavefrontPool(4)
	wf := wavefront.NewWavefront(kernels.NewWavefront())
	inst := wavefront.NewInst(insts.NewInst())
	inst.ExeUnit = insts.ExeUnitVALU
	wf.SetDynamicInst(inst)
	wf.State = wavefront.WfRunning
	pool.AddWf(wf)
	cu.WfPools = []*WavefrontPool{pool}

	cu.logInstTask(wf, inst, false)
	if rec.NumStarted() != 1 {
		t.Fatalf("expected the inst task to be started, got %d", rec.NumStarted())
	}

	cu.endInflightTracingTasks()

	if got := rec.OpenSummary(); got != "none" {
		t.Fatalf("flush leaked tracing tasks: %s", got)
	}
}

// A WfReady wavefront (e.g. one whose memory access is in flight and preserved
// in the shadow buffer) must be left alone — its inst task is ended when the
// shadowed response returns, so the flush sweep must not end it.
func TestEndInflightTracingTasksSkipsNonRunningWavefronts(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(cu.comp, rec)

	pool := NewWavefrontPool(4)
	wf := wavefront.NewWavefront(kernels.NewWavefront())
	inst := wavefront.NewInst(insts.NewInst())
	inst.ExeUnit = insts.ExeUnitVMem
	wf.SetDynamicInst(inst)
	wf.State = wavefront.WfReady
	pool.AddWf(wf)
	cu.WfPools = []*WavefrontPool{pool}

	cu.logInstTask(wf, inst, false)

	cu.endInflightTracingTasks()

	if len(rec.OpenTasks()) != 1 {
		t.Fatalf("the WfReady wavefront's inst task should stay open, got %s",
			rec.OpenSummary())
	}
}

// In-flight memory accesses are re-sent from the shadow buffers with freshly
// generated message IDs, so their original req_out tasks would never be
// finalized. populateShadowBuffers must end them.
func TestPopulateShadowBuffersEndsReqOutTasks(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(cu.comp, rec)

	fetchTaskID := timing.GetIDGenerator().Generate()
	tracing.StartTask(cu.comp, tracing.TaskStart{
		ID: fetchTaskID, Kind: "fetch", What: "fetch",
	})
	req := memprotocol.ReadReq{
		MsgMeta: messaging.MsgMeta{ID: timing.GetIDGenerator().Generate()},
	}
	tracing.TraceReqInitiate(cu.comp, req, fetchTaskID)

	cu.InFlightInstFetch = []*InstFetchReqInfo{
		{Req: req, FetchTaskID: fetchTaskID},
	}

	cu.populateShadowBuffers()

	for _, open := range rec.OpenTasks() {
		if open.Kind == tracing.ReqOutTaskKind {
			t.Fatalf("populateShadowBuffers leaked a req_out task")
		}
	}
}

// The shadow resend assigns a fresh message ID, so it must open a replacement
// req_out task (the original was ended in populateShadowBuffers); otherwise the
// post-flush memory round trip has no task and TraceReqFinalize finds nothing.
func TestShadowResendOpensReplacementReqOut(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	cu.ToInstMem = newFakePort("CU.ToInstMem")
	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(cu.comp, rec)

	cu.shadowInFlightInstFetch = []*InstFetchReqInfo{
		{
			Req: memprotocol.ReadReq{
				MsgMeta: messaging.MsgMeta{
					ID: timing.GetIDGenerator().Generate(),
				},
			},
			FetchTaskID: timing.GetIDGenerator().Generate(),
		},
	}

	cu.sendInstFetchShadowBufferAccesses()

	resentID := cu.InFlightInstFetch[0].Req.ID
	found := false
	for _, open := range rec.OpenTasks() {
		if open.Kind == tracing.ReqOutTaskKind && open.ID == resentID {
			found = true
		}
	}
	if !found {
		t.Fatalf("shadow resend should open a req_out task for the resent "+
			"fetch; open=%s", rec.OpenSummary())
	}
}

// The per-SIMD "pipeline" tasks live on the SIMD-unit tracing domain (so the
// per-SIMD busy-time tracer can measure them). A flush drops the instructions
// in the SIMD pipeline, so those pipeline tasks must be ended too.
func TestSIMDFlushEndsPipelineTasks(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	simd := NewSIMDUnit(cu, "CU.SIMD", new(mockALU))
	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(simd, rec)

	wave := wavefront.NewWavefront(kernels.NewWavefront())
	inst := wavefront.NewInst(insts.NewInst())
	inst.ExeUnit = insts.ExeUnitVALU
	wave.SetDynamicInst(inst)

	simd.AcceptWave(wave)
	if rec.NumStarted() != 1 {
		t.Fatalf("expected the pipeline task to be started, got %d",
			rec.NumStarted())
	}

	simd.Flush()

	if got := rec.OpenSummary(); got != "none" {
		t.Fatalf("SIMD flush leaked tracing tasks: %s", got)
	}
}
