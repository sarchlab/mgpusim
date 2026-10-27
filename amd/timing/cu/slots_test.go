package cu

import (
	"bytes"
	"fmt"
	"slices"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
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
	old.execution().State = wavefront.WfDraining
	old.execution().IsFetching = true
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
	w.execution().State = wavefront.WfDraining
	w.execution().IsFetching = true
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
	if live.execution().State != wavefront.WfDraining {
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
	c, simulation := checkpointCUAndSimulation(t, false)
	g := checkpointGroup(c)
	w := g.waves()[0]
	port := newFakePort("CU.ACE")
	port.full = true
	c.ToACE = port
	w.execution().State = wavefront.WfSampledWaiting
	c.scheduleSampledCompletion(0, w.handle)
	c.Tick()
	if len(port.sent) != 0 || w.execution().State != wavefront.WfCompleted {
		t.Fatal("sampled completion did not remain pending")
	}
	// Completion must arrange a regular tick even though no instruction or
	// memory work remains. Run the actual serial engine after freeing the port.
	port.full = false
	if err := simulation.Engine().Run(); err != nil {
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
	for _, tracking := range []bool{false, true} {
		t.Run(fmt.Sprint(tracking), func(t *testing.T) { checkFinalInstructionOrder(t, tracking) })
	}
}
func checkFinalInstructionOrder(t *testing.T, tracking bool) {
	t.Helper()
	c := checkpointCUWithTracking(t, tracking)
	g := checkpointGroup(c)
	w := g.waves()[0]
	c.WfPools[0].AddWf(w)
	instruction := checkpointInstruction(t, c)
	instruction.Raw = []byte{0, 0, 0x81, 0xbf}
	w.setTestInst(instruction)
	w.execution().State = wavefront.WfRunning
	startTestWave(c, w)
	c.beginInstruction(w, w.InstID())
	recorder := &finalWaveOrderRecorder{inst: c.comp.State.InstTasks[instruction.ID], wave: c.comp.State.WFTasks[w.UID]}
	if tracking {
		c.comp.TaskTracker().Subscribe(recorder, c.comp)
	}
	port := newFakePort("CU.ACE")
	port.full = true
	c.ToACE = port
	scheduler := c.Scheduler.(*SchedulerImpl)
	scheduler.state().Internal = []waveHandle{w.handle}
	scheduler.EvaluateInternalInst()
	expected := 0
	if tracking {
		expected = 2
	}
	if len(recorder.ended) != expected || (tracking &&
		(recorder.ended[0] != recorder.inst || recorder.ended[1] != recorder.wave)) ||
		w.execution().InFlightInsts != 0 || c.comp.State.Instructions[instruction.ID].ExecutionPending {
		t.Fatalf("incorrect final lifetime order/count: %v", recorder.ended)
	}
	c.retireGroups()
	c.retireGroups()
	port.full = false
	c.retireGroups()
	c.retireGroups()
	if len(recorder.ended) != expected || len(port.sent) != 1 {
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

func TestSampledCompletionPreservesResidentRegistersDuringBackpressure(t *testing.T) {
	c := checkpointCU(t)
	resident := checkpointGroup(c).waves()[0]
	resident.execution().State = wavefront.WfRunning
	c.WfPools[0].AddWf(resident)
	sampledGroup := checkpointGroup(c)
	sampledGroup.state().Raw.CodeObject.Meta.WIVgprCount = 4
	sampledGroup.state().Raw.CodeObject.Meta.WFSgprCount = 4
	sampled := sampledGroup.waves()[0]
	sampled.execution().State = wavefront.WfSampledWaiting
	scalar := bytes.Repeat([]byte{0x35}, len(c.comp.State.SRegisters))
	copy(c.comp.State.SRegisters, scalar)
	vectors := make([][]byte, len(c.comp.State.VRegisters))
	for i := range vectors {
		vectors[i] = bytes.Repeat([]byte{byte(i + 0x51)}, len(c.comp.State.VRegisters[i]))
		copy(c.comp.State.VRegisters[i], vectors[i])
	}
	unchanged := func() bool {
		return bytes.Equal(scalar, c.comp.State.SRegisters) && slices.EqualFunc(vectors, c.comp.State.VRegisters, bytes.Equal)
	}
	port := newFakePort("CU.ACE")
	port.full = true
	c.ToACE = port
	c.scheduleSampledCompletion(0, sampled.handle)
	c.completeSampledWaves()
	c.retireGroups()
	if !unchanged() || len(port.sent) != 0 || resident.execution().State != wavefront.WfRunning {
		t.Fatal("sampled completion changed resident registers while ACE was full")
	}
	port.full = false
	c.retireGroups()
	c.retireGroups()
	if !unchanged() || len(port.sent) != 1 || liveTestGroups(&c.comp.State) != 1 {
		t.Fatal("sampled completion retry changed resident registers or delivered more than once")
	}
}

func TestTypedSampledCompletionSurvivesRestoreAndRejectsRecycledSlot(t *testing.T) {
	c := checkpointCU(t)
	w := checkpointGroup(c).waves()[0]
	w.execution().State = wavefront.WfSampledWaiting
	completion := SampledCompletion{Wave: w.handle}
	c.comp.State.SampledCompletions = []SampledCompletion{completion}
	var handle wavefront.Handle = completion.Wave
	restored := roundTripCUState(t, c)
	if restored.wave(handle).UID != w.UID {
		t.Fatal("typed local handle lost global identity across restore")
	}
	restored.ToACE = newFakePort("CU.ACE")
	restored.completeSampledWaves()
	restored.retireGroups()
	next := checkpointGroup(restored).waves()[0]
	if slotIndex(next.handle) != slotIndex(handle) || next.handle == handle {
		t.Fatal("slot was not recycled with a new generation")
	}
	restored.comp.State.SampledCompletions = []SampledCompletion{completion}
	requireSlotPanic(t, func() { restored.completeSampledWaves() })
	if next.execution().State != wavefront.WfDispatching {
		t.Fatal("stale sampled event affected new occupant")
	}
}
