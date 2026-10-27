package cu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

func requireSlotPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("stale/exhausted slot was accepted")
		}
	}()
	f()
}

func TestFixedHardwareSlotsRetainDrainingWavesAndRejectStaleHandles(t *testing.T) {
	spec := DefaultSpec()
	spec.WfPoolSize = 1
	c := checkpointCU(t, spec)
	wavesBase, groupsBase := &c.comp.State.Waves[0], &c.comp.State.Groups[0]
	var groups []*WorkGroup
	for range numWfPools {
		groups = append(groups, checkpointGroup(c))
	}
	capacity := numWfPools * spec.WfPoolSize
	if len(c.comp.State.Waves) != capacity || len(c.comp.State.Groups) != capacity {
		t.Fatal("incorrect fixed hardware capacity")
	}
	old := groups[1].waves()[0]
	old.execution().State = wavefront.WfCompleted
	old.state().StopFetch = true
	c.WfPools[0].AddWf(old)
	port := newFakePort("CU.ACE")
	c.ToACE = port
	req := messaging.Msg{ID: c.comp.NewID(), Payload: memprotocol.ReadReq{AccessByteSize: 4}}
	c.comp.State.InstFetch = []FetchState{{Req: req, WF: old.handle}}
	c.retireGroups()
	requireSlotPanic(t, func() { checkpointGroup(c) })
	if len(port.sent) != 0 || liveTestWaves(&c.comp.State) != capacity || c.WfPools[0].Availability() != 0 {
		t.Fatal("draining wave released capacity")
	}
	c.handleFetchReturn(messaging.Msg{RspTo: req.ID, Payload: memprotocol.DataReadyRsp{Data: []byte{0, 0, 0, 0}}})
	c.retireGroups()
	if len(port.sent) != 1 || c.WfPools[0].Availability() != 1 {
		t.Fatal("drained wave did not release capacity")
	}
	if port.sent[0].Payload.(protocol.WGCompletionMsg).Timings[0].WavefrontID != old.UID {
		t.Fatal("protocol exposed local slot instead of global UID")
	}
	checkRepeatedSlotReuse(t, c, old, groups[1])
	if &c.comp.State.Waves[0] != wavesBase || &c.comp.State.Groups[0] != groupsBase ||
		len(c.comp.State.Waves) != capacity || len(c.comp.State.Groups) != capacity {
		t.Fatal("slot storage resized or moved")
	}
	if groups[0].slot != 0 || groups[2].slot != 2 || groups[3].slot != 3 {
		t.Fatal("surviving groups were compacted")
	}
}

func TestCheckpointDuringDrainThenSlotReuse(t *testing.T) {
	c := checkpointCU(t)
	g := checkpointGroup(c)
	w := g.waves()[0]
	w.execution().State = wavefront.WfCompleted
	w.state().StopFetch = true
	c.WfPools[0].AddWf(w)
	req := messaging.Msg{ID: c.comp.NewID(), Payload: memprotocol.ReadReq{AccessByteSize: 4}}
	c.comp.State.InstFetch = []FetchState{{Req: req, WF: w.handle}}
	restored := roundTripCUState(t, c)
	live := restored.wave(w.handle)
	port := newFakePort("CU.ACE")
	restored.ToACE = port
	restored.retireGroups()
	if len(port.sent) != 0 || !live.state().StopFetch {
		t.Fatal("restore released or revived draining wave")
	}
	replaceCUState(t, restored)
	if live.execution().State != wavefront.WfCompleted {
		t.Fatal("pre-replacement handle lost identity")
	}
	restored.handleFetchReturn(messaging.Msg{RspTo: req.ID, Payload: memprotocol.DataReadyRsp{Data: []byte{0, 0, 0, 0}}})
	restored.retireGroups()
	next := checkpointGroup(restored).waves()[0]
	requireSlotPanic(t, func() { live.PC() })
	again := roundTripCUState(t, restored)
	if again.wave(next.handle).UID != next.UID {
		t.Fatal("reused slot identity did not survive checkpoint")
	}
	requireSlotPanic(t, func() { again.wave(w.handle) })
}

func TestSampledCompletionBackpressureSchedulesRetryExactlyOnce(t *testing.T) {
	c := checkpointCU(t)
	g := checkpointGroup(c)
	w := g.waves()[0]
	port := newFakePort("CU.ACE")
	port.full = true
	c.ToACE = port
	w.execution().State = wavefront.WfSampledCompleted
	c.Handle(wavefront.NewWfCompletionEvent(c.comp.NewID(), 0, c.wfCompletionHandlerID, w.handle))
	if len(port.sent) != 0 || w.execution().State != wavefront.WfCompleted {
		t.Fatal("sampled completion did not remain pending")
	}
	// The custom event must arrange a regular tick even though no instruction or
	// memory work remains. Run the actual serial engine after freeing the port.
	port.full = false
	if err := c.engine.(timing.Engine).Run(); err != nil {
		t.Fatal(err)
	}
	if len(port.sent) != 1 || liveTestGroups(&c.comp.State) != 0 {
		t.Fatal("sampled retry did not finish exactly once")
	}
	c.retireGroups()
	if len(port.sent) != 1 {
		t.Fatal("duplicate work-group completion")
	}
}

func TestStopFetchSurvivesFlushWhileEndingAndWhileDraining(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(map[bool]string{false: "waiting-data", true: "terminal-fetch"}[terminal], func(t *testing.T) {
			checkStopFetchFlush(t, terminal)
		})
	}
}

// The observer must see one instruction completion before its enclosing wave
// closes. CPI tracing can then include the final S_ENDPGM interval.
type finalWaveOrderRecorder struct {
	tasks.NopObserver
	inst, wave uint64
	ended      []uint64
}

func (r *finalWaveOrderRecorder) TaskEnded(task tasks.Task) {
	if task.ID == r.inst || task.ID == r.wave {
		r.ended = append(r.ended, task.ID)
	}
}
func TestFinalInstructionClosesBeforeWaveExactlyOnce(t *testing.T) {
	c := checkpointCUWithTracking(t, true)
	g := checkpointGroup(c)
	w := g.waves()[0]
	c.WfPools[0].AddWf(w)
	instruction := checkpointInstruction(t, c)
	instruction.Raw = []byte{0, 0, 0x81, 0xbf}
	w.SetDynamicInst(instruction)
	w.execution().State = wavefront.WfRunning
	startTestWave(c, w)
	c.logInstTask(w, w.DynamicInst(), false)
	recorder := &finalWaveOrderRecorder{inst: c.comp.State.InstTasks[instruction.ID], wave: c.comp.State.WFTasks[w.UID]}
	c.comp.TaskTracker().Subscribe(recorder, c.comp)
	port := newFakePort("CU.ACE")
	port.full = true
	c.ToACE = port
	scheduler := c.Scheduler.(*SchedulerImpl)
	scheduler.state().Internal = []uint64{w.handle}
	scheduler.EvaluateInternalInst()
	if len(recorder.ended) != 2 || recorder.ended[0] != recorder.inst ||
		recorder.ended[1] != recorder.wave || w.execution().InFlightInsts != 0 {
		t.Fatalf("incorrect final lifetime order/count: %v", recorder.ended)
	}
	c.retireGroups()
	c.retireGroups()
	port.full = false
	c.retireGroups()
	c.retireGroups()
	if len(recorder.ended) != 2 || len(port.sent) != 1 {
		t.Fatal("ACE retries repeated completion")
	}
	if len(c.comp.State.Scheduler.Internal) != 0 || len(c.comp.State.Scheduler.Barrier) != 0 {
		t.Fatal("retired wave retained scheduling references")
	}
}

func BenchmarkFixedSlotCompletionScan(b *testing.B) {
	for _, occupied := range []bool{false, true} {
		b.Run(map[bool]string{false: "empty", true: "one-active"}[occupied], func(b *testing.B) {
			c := newTestComputeUnit("CU", nil)
			if occupied {
				w := newTestWave()
				registerTestWave(c, w)
				w.execution().State = wavefront.WfRunning
			}
			b.ResetTimer()
			for b.Loop() {
				c.retireGroups()
			}
		})
	}
}

func checkRepeatedSlotReuse(t *testing.T, c *ComputeUnit, old *Wavefront, group *WorkGroup) {
	t.Helper()
	for range 20 {
		g := checkpointGroup(c)
		w := g.waves()[0]
		if slotIndex(w.handle) != slotIndex(old.handle) || w.handle == old.handle || w.UID == old.UID {
			t.Fatal("slot reuse did not preserve identity distinction")
		}
		requireSlotPanic(t, func() { old.SetPC(123) })
		requireSlotPanic(t, func() { c.wave(old.handle) })
		requireSlotPanic(t, func() { group.state() })
		w.execution().State = wavefront.WfCompleted
		c.retireGroups()
	}
}

func checkStopFetchFlush(t *testing.T, terminal bool) {
	t.Helper()
	c := checkpointCU(t)
	g := checkpointGroup(c)
	w := g.waves()[0]
	c.WfPools[0].AddWf(w)
	inst := checkpointInstruction(t, c)
	w.SetDynamicInst(inst)
	w.execution().InstBuffer = []byte{0, 0, 0x81, 0xbf} // s_endpgm
	w.execution().State = wavefront.WfRunning
	w.execution().OutstandingScalarMemAccess = 1
	scheduler := c.Scheduler.(*SchedulerImpl)
	scheduler.state().Internal = []uint64{w.handle}
	if progress, _ := scheduler.evalSEndPgm(w); progress {
		t.Fatal("did not wait for data")
	}
	if terminal {
		w.execution().State = wavefront.WfCompleted
		w.execution().OutstandingScalarMemAccess = 0
	}
	req := messaging.Msg{ID: c.comp.NewID(), Payload: memprotocol.ReadReq{AccessByteSize: 4}}
	c.comp.State.InstFetch = []FetchState{{Req: req, WF: w.handle}}
	w.execution().IsFetching = true
	c.comp.State.HasFlushReq = true
	if !c.flushPipeline() {
		t.Fatal("flush failed")
	}
	if !w.state().StopFetch || (&FetchArbiter{InstBufByteSize: 256}).canFetchFromWF(w) {
		t.Fatal("flush restarted fetching after end")
	}
	if terminal && w.execution().State != wavefront.WfCompleted {
		t.Fatal("flush revived terminal wave")
	}
	if len(c.comp.State.ShadowInstFetch) != 1 || len(c.comp.State.InstFetch) != 0 {
		t.Fatal("flush lost fetch reference")
	}
	port := newFakePort("CU.ACE")
	c.ToACE = port
	c.retireGroups()
	if len(port.sent) != 0 {
		t.Fatal("shadow fetch allowed early completion")
	}
	checkRestartAfterEnd(t, c, w, terminal)
}

func checkRestartAfterEnd(t *testing.T, c *ComputeUnit, w *Wavefront, terminal bool) {
	t.Helper()
	scheduler := c.Scheduler.(*SchedulerImpl)
	c.ToInstMem = newFakePort("CU.Inst")
	if !c.sendInstFetchShadowBufferAccesses() {
		t.Fatal("shadow fetch did not resend")
	}
	c.checkShadowBuffers()
	if !w.state().StopFetch || (&FetchArbiter{InstBufByteSize: 256}).canFetchFromWF(w) {
		t.Fatal("restart resumed fetching")
	}
	if !terminal {
		w.execution().OutstandingScalarMemAccess = 0
		if !scheduler.DecodeNextInst() {
			t.Fatal("ending instruction could not resume after flush")
		}
		if w.NextInst().Opcode != 1 {
			t.Fatal("resumed wrong instruction")
		}
	}
}
