package cu

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

func lifecycleInstruction(t *testing.T, c *ComputeUnit, raw []byte) *testInstruction {
	t.Helper()
	decoded, err := c.Decoder.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return &testInstruction{ID: c.comp.NewID(), Inst: decoded, Raw: raw}
}

func lifecycleGroup(c *ComputeUnit, waves int) *WorkGroup {
	grid := kernels.NewGridBuilder(c.comp)
	grid.SetKernel(kernels.KernelLaunchInfo{
		CodeObject: &insts.KernelCodeObject{KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{}},
		Packet: &kernels.HsaKernelDispatchPacket{GridSizeX: uint32(64 * waves), GridSizeY: 1, GridSizeZ: 1,
			WorkgroupSizeX: uint16(64 * waves), WorkgroupSizeY: 1, WorkgroupSizeZ: 1},
	})
	return c.wrapWG(grid.NextWG(), messaging.Msg{ID: c.comp.NewID(), Payload: protocol.MapWGReq{}})
}

func TestEndProgramWaitsForDataThenPrefetchWithoutRevivingWave(t *testing.T) {
	for _, tracking := range []bool{false, true} {
		t.Run(fmt.Sprint(tracking), func(t *testing.T) { checkEndProgramDrain(t, tracking) })
	}
}

func checkEndProgramDrain(t *testing.T, tracking bool) {
	t.Helper()
	c := checkpointCUWithTracking(t, tracking)
	w := checkpointGroup(c).waves()[0]
	c.WfPools[0].AddWf(w)
	w.setTestInst(lifecycleInstruction(t, c, []byte{0, 0, 0x81, 0xbf}))
	w.execution().State = wavefront.WfRunning
	c.beginInstruction(w, w.InstID())
	scheduler := c.Scheduler.(*SchedulerImpl)
	scheduler.state().Internal = []waveHandle{w.handle}
	fetch := newFakePort("CU.InstMem")
	c.ToInstMem = fetch
	// Issue a genuine prefetch before S_ENDPGM observes the outstanding data.
	if !scheduler.DoFetch() || len(fetch.sent) != 1 {
		t.Fatal("prefetch was not issued")
	}
	w.execution().OutstandingScalarMemAccess = 1
	if scheduler.EvaluateInternalInst() || !w.state().StopFetch {
		t.Fatal("end-program did not wait for data")
	}
	if scheduler.DoFetch() || len(fetch.sent) != 1 {
		t.Fatal("end-program fetched again while waiting for data")
	}
	replaceCUState(t, c)
	w.execution().OutstandingScalarMemAccess = 0
	if !scheduler.EvaluateInternalInst() || w.execution().State != wavefront.WfDraining {
		t.Fatal("end-program did not enter prefetch drain")
	}
	checkDrainCompletion(t, c, w, fetch.sent[0].ID)
}

func checkDrainCompletion(t *testing.T, c *ComputeUnit, w *Wavefront, requestID uint64) {
	t.Helper()
	scheduler := c.Scheduler.(*SchedulerImpl)
	// Trace accounting is deliberately not a lifecycle input.
	w.execution().InFlightInsts = -7
	assertLifecycleHolders(t, c)
	ace := newFakePort("CU.ACE")
	ace.full = true
	c.ToACE = ace
	c.retireGroups()
	if scheduler.DoFetch() || len(ace.sent) != 0 {
		t.Fatal("draining wave fetched or released resources")
	}
	c.handleFetchReturn(messaging.Msg{RspTo: requestID, Payload: memprotocol.DataReadyRsp{Data: make([]byte, 64)}})
	if w.execution().State != wavefront.WfCompleted {
		t.Fatal("final fetch did not finish draining immediately")
	}
	assertLifecycleHolders(t, c)
	c.retireGroups()
	c.retireGroups()
	if liveTestGroups(&c.comp.State) != 1 {
		t.Fatal("ACE backpressure released slots")
	}
	ace.full = false
	c.retireGroups()
	c.retireGroups()
	assertLifecycleHolders(t, c)
	if len(ace.sent) != 1 || liveTestWaves(&c.comp.State) != 0 {
		t.Fatal("completion was not exactly once")
	}
}

func TestAsyncInstructionLifetimeSurvivesAdvanceAndRestore(t *testing.T) {
	for _, kind := range []string{"scalar", "load", "store"} {
		for _, tracking := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tracking=%t", kind, tracking), func(t *testing.T) { checkAsyncInstruction(t, kind, tracking) })
		}
	}
}

func checkAsyncInstruction(t *testing.T, kind string, tracking bool) {
	t.Helper()
	c := checkpointCUWithTracking(t, tracking)
	w := checkpointGroup(c).waves()[0]
	inst, first, second := prepareAsyncInstruction(t, c, w, kind)
	probe := &asyncCompletionProbe{c: c, id: inst.ID, task: c.comp.State.InstTasks[inst.ID]}
	if tracking {
		c.comp.TaskTracker().Subscribe(probe, c.comp)
	}
	w.execution().State = wavefront.WfReady
	// Advance many instructions while an old async operation still owns its encoding.
	for range 128 {
		w.setTestNextInst(checkpointInstruction(t, c))
		w.issueNext()
		c.beginInstruction(w, w.InstID())
		c.completeInstruction(w, w.InstID())
		if len(c.comp.State.Instructions) != 2 {
			t.Fatal("instruction storage was lost or grew without bound")
		}
	}
	c.rememberInstruction(w.handle, inst.saved())
	if !tracking {
		delete(c.comp.State.InstTasks, inst.ID)
	}
	if !c.comp.State.Instructions[inst.ID].ExecutionPending {
		t.Fatal("re-registration reset execution state")
	}
	assertLifecycleHolders(t, c)
	if !tracking {
		c = roundTripCUState(t, c)
		w = c.wave(w.handle)
	}
	replaceCUState(t, c)
	assertLifecycleHolders(t, c)
	completeAsyncInstruction(t, c, w, kind, inst.ID, first, second)
	if tracking && probe.calls != 1 {
		t.Fatal("async completion callback was not exactly once")
	}
}

func prepareAsyncInstruction(t *testing.T, c *ComputeUnit, w *Wavefront, kind string) (
	*testInstruction, messaging.Msg, messaging.Msg,
) {
	t.Helper()
	raw := []byte{0, 0, 0x50, 0xdc, 0, 0, 0, 0} // flat_load_dword
	if kind == "scalar" {
		raw = []byte{0, 0, 0, 0xc0, 0, 0, 0, 0}
	} // s_load_dword
	if kind == "store" {
		raw = []byte{0, 0, 0x70, 0xdc, 0, 0, 0, 0}
	} // flat_store_dword
	inst := lifecycleInstruction(t, c, raw)
	w.setTestInst(inst)
	c.beginInstruction(w, w.InstID())
	first := messaging.Msg{ID: c.comp.NewID(), Payload: memprotocol.ReadReq{AccessByteSize: 4}}
	second := first
	second.ID = c.comp.NewID()
	if kind == "scalar" {
		w.execution().OutstandingScalarMemAccess = 1
		c.comp.State.Scalar = []ScalarAccessState{
			{Req: first, WF: w.handle, Inst: inst.ID, Register: insts.SReg(0).RegType},
			{Req: second, WF: w.handle, Inst: inst.ID, Register: insts.SReg(1).RegType},
		}
	} else {
		w.execution().OutstandingVectorMemAccess = 1
		w.execution().OutstandingScalarMemAccess = 1
		for _, req := range []messaging.Msg{first, second} {
			record := VectorAccessState{WF: w.handle, Inst: inst.ID,
				Lanes: []LaneState{{Register: insts.VReg(0).RegType, Count: 1}}}
			if kind == "store" {
				record.Write = req
			} else {
				record.Read = req
			}
			c.comp.State.Vector = append(c.comp.State.Vector, record)
		}
	}
	return inst, first, second
}

func completeAsyncInstruction(t *testing.T, c *ComputeUnit, w *Wavefront, kind string,
	instructionID uint64, first, second messaging.Msg) {
	t.Helper()
	respond := func(req messaging.Msg) {
		rsp := messaging.Msg{RspTo: req.ID, Payload: memprotocol.DataReadyRsp{Data: []byte{43, 0, 0, 0}}}
		switch kind {
		case "scalar":
			c.handleScalarDataLoadReturn(rsp)
		case "load":
			c.handleVectorDataLoadReturn(rsp)
		case "store":
			c.handleVectorDataStoreRsp(rsp)
		}
	}
	respond(second)
	if !c.comp.State.Instructions[instructionID].ExecutionPending {
		t.Fatal("first response retired instruction before its sibling")
	}
	respond(first)
	if _, ok := c.comp.State.Instructions[instructionID]; ok {
		t.Fatal("completed async instruction leaked")
	}
	if w.execution().OutstandingScalarMemAccess != 0 || w.execution().OutstandingVectorMemAccess != 0 {
		t.Fatal("memory counts did not drain")
	}
	if kind == "scalar" && c.comp.State.SRegisters[0] != 43 {
		t.Fatal("restored scalar response lost data")
	}
	if kind == "load" && c.comp.State.VRegisters[0][0] != 43 {
		t.Fatal("restored vector response lost data")
	}
	assertLifecycleHolders(t, c)
}

func TestSampledWaitingSurvivesCheckpointAndPartialCompletion(t *testing.T) {
	c, simulation := checkpointCUAndSimulation(t, false)
	g := lifecycleGroup(c, 2)
	for i, w := range g.waves() {
		w.execution().State = wavefront.WfSampledWaiting
		c.scheduleSampledCompletion(timing.VTimeInPicoSec(1000+i*1000), w.handle)
	}
	assertLifecycleHolders(t, c)
	if len(testPendingCompletions(t, c)) != 2 {
		t.Fatal("sampled event audit missed queued waves")
	}
	var archive bytes.Buffer
	if err := simulation.Engine().(*timing.SerialEngine).SaveCheckpoint(&archive); err != nil {
		t.Fatal(err)
	}
	var state bytes.Buffer
	if err := c.comp.SaveCheckpoint(&state); err != nil {
		t.Fatal(err)
	}
	restored, restoredSimulation := checkpointCUAndSimulation(t, false)
	if err := restored.comp.LoadCheckpoint(&state); err != nil {
		t.Fatal(err)
	}
	if err := restoredSimulation.Engine().(*timing.SerialEngine).LoadCheckpoint(&archive); err != nil {
		t.Fatal(err)
	}
	assertLifecycleHolders(t, restored)
	ace := newFakePort("CU.ACE")
	ace.full = true
	restored.ToACE = ace
	if err := restoredSimulation.Engine().(*timing.SerialEngine).RunUntil(1500); err != nil {
		t.Fatal(err)
	}
	if restored.wave(g.state().Waves[0]).execution().State != wavefront.WfCompleted ||
		restored.wave(g.state().Waves[1]).execution().State != wavefront.WfSampledWaiting {
		t.Fatal("partial sampled completion released peers")
	}
	assertLifecycleHolders(t, restored)
	if err := restoredSimulation.Engine().(*timing.SerialEngine).RunUntil(2500); err != nil {
		t.Fatal(err)
	}
	assertLifecycleHolders(t, restored)
	if liveTestWaves(&restored.comp.State) != 2 || len(ace.sent) != 0 {
		t.Fatal("sampled completion bypassed ACE backpressure")
	}
	ace.full = false
	if err := restoredSimulation.Engine().Run(); err != nil {
		t.Fatal(err)
	}
	assertLifecycleHolders(t, restored)
	if len(ace.sent) != 1 || liveTestWaves(&restored.comp.State) != 0 {
		t.Fatal("sampled completion did not release exactly once")
	}
}

func TestPipelineLifecycleHoldersAtEveryTick(t *testing.T) {
	for _, scoreboard := range []bool{false, true} {
		t.Run(fmt.Sprint(scoreboard), func(t *testing.T) { checkPipelineLifecycle(t, scoreboard) })
	}
}
func checkPipelineLifecycle(t *testing.T, scoreboard bool) {
	t.Helper()
	spec := DefaultSpec()
	spec.RegisterScoreboard = scoreboard
	c := checkpointCU(t, spec)
	w := checkpointGroup(c).waves()[0]
	// Scalar/vector ALU, async scalar/vector loads, then s_endpgm.
	code := []byte{0x81, 0, 0x80, 0xbe, 0x81, 2, 0, 0x7e,
		0, 0, 0, 0xc0, 0, 0, 0, 0, 0, 0, 0x50, 0xdc, 0, 0, 0, 0, 0, 0, 0x81, 0xbf}
	c.comp.Resources.VectorMemModules = mem.AddressMapper{Destinations: []messaging.RemotePort{"VectorMem"}}
	scalar, vector := newFakePort("CU.ScalarMem"), newFakePort("CU.VectorMem")
	scalar.full = true
	vector.full = true
	c.ToScalarMem = scalar
	c.ToVectorMem = vector
	w.execution().InstBuffer = code
	w.execution().State = wavefront.WfReady
	w.SetEXEC(^uint64(0))
	w.group().raw().CodeObject.HasSymbol = true
	w.group().raw().CodeObject.Symbol.Size = uint64(len(code))
	c.WfPools[0].AddWf(w)
	ace := newFakePort("CU.ACE")
	ace.full = true
	c.ToACE = ace
	seen := runLifecyclePipeline(t, c, w, scalar, vector)
	simd := "SIMD"
	if scoreboard {
		simd = "SIMD slot"
	}
	for _, kind := range []string{"decoder", "read stage", "exec stage", "write stage", simd, "internal",
		"scalar request buffer", "vector instruction pipeline", "vector instruction output",
		"transaction pipeline", "transaction output", "coalescing"} {
		if !seen[kind] {
			t.Fatalf("test did not traverse %s", kind)
		}
	}
	if w.execution().State != wavefront.WfCompleted {
		t.Fatal("pipeline did not reach drained state")
	}
	ace.full = false
	c.Tick()
	assertLifecycleHolders(t, c)
	if len(ace.sent) != 1 || len(c.comp.State.Instructions) != 0 {
		t.Fatal("pipeline lifetime leaked")
	}
}
func runLifecyclePipeline(t *testing.T, c *ComputeUnit, w *Wavefront, scalar, vector *fakePort) map[string]bool {
	t.Helper()
	scalarSent, vectorSent := 0, 0
	seen := make(map[string]bool)
	for cycle := 0; cycle < 200 && w.execution().State != wavefront.WfCompleted; cycle++ {
		c.Tick()
		assertLifecycleHolders(t, c)
		for _, holders := range testOperationHolders(c) {
			for _, kind := range holders {
				seen[kind] = true
			}
		}
		if cycle == 4 {
			replaceCUState(t, c)
		}
		if seen["scalar request buffer"] {
			scalar.full = false
		}
		if seen["transaction output"] {
			vector.full = false
		}
		scalarSent = deliverLifecycleReads(scalar, scalarSent)
		vectorSent = deliverLifecycleReads(vector, vectorSent)
	}
	return seen
}

func TestBarrierPeersTreatDrainingWaveAsExecutionEnded(t *testing.T) {
	c := checkpointCU(t)
	g := lifecycleGroup(c, 3)
	waves := g.waves()
	for _, w := range waves {
		c.WfPools[0].AddWf(w)
	}
	scheduler := c.Scheduler.(*SchedulerImpl)
	end := func(w *Wavefront) {
		w.setTestInst(lifecycleInstruction(t, c, []byte{0, 0, 0x81, 0xbf}))
		w.execution().State = wavefront.WfRunning
		c.beginInstruction(w, w.InstID())
		scheduler.state().Internal = []waveHandle{w.handle}
		scheduler.EvaluateInternalInst()
	}
	req := messaging.Msg{ID: c.comp.NewID(), Payload: memprotocol.ReadReq{AccessByteSize: 4}}
	c.comp.State.InstFetch = []FetchState{{Req: req, WF: waves[0].handle}}
	waves[0].execution().IsFetching = true
	end(waves[0])
	barrier := waves[1]
	barrier.setTestInst(lifecycleInstruction(t, c, []byte{0, 0, 0x8a, 0xbf}))
	barrier.execution().State = wavefront.WfRunning
	c.beginInstruction(barrier, barrier.InstID())
	scheduler.state().Internal = []waveHandle{barrier.handle}
	scheduler.EvaluateInternalInst()
	assertLifecycleHolders(t, c)
	if barrier.execution().State != wavefront.WfAtBarrier {
		t.Fatal("barrier did not wait for live peer")
	}
	end(waves[2])
	if waves[0].execution().State != wavefront.WfDraining || barrier.execution().State != wavefront.WfReady {
		t.Fatal("end-program revived draining peer or failed to release barrier")
	}
	assertLifecycleHolders(t, c)
	c.handleFetchReturn(messaging.Msg{RspTo: req.ID, Payload: memprotocol.DataReadyRsp{Data: make([]byte, 4)}})
	end(barrier)
	assertLifecycleHolders(t, c)
	ace := newFakePort("CU.ACE")
	c.ToACE = ace
	c.retireGroups()
	assertLifecycleHolders(t, c)
	if len(ace.sent) != 1 {
		t.Fatal("multiwave completion did not retire")
	}
}

func deliverLifecycleReads(port *fakePort, start int) int {
	for _, req := range port.sent[start:] {
		size := req.Payload.(memprotocol.ReadReq).AccessByteSize
		port.incoming = append(port.incoming, messaging.Msg{RspTo: req.ID,
			Payload: memprotocol.DataReadyRsp{Data: make([]byte, size)}})
	}
	return len(port.sent)
}

// A completion callback must still see the pending instruction even after the
// wave has advanced. Deletion is allowed only after this callback returns.
type asyncCompletionProbe struct {
	tasks.NopObserver
	c        *ComputeUnit
	id, task uint64
	calls    int
}

func (p *asyncCompletionProbe) TaskEnded(task tasks.Task) {
	if task.ID != p.task {
		return
	}
	p.c.instruction(p.id)
	if !p.c.comp.State.Instructions[p.id].ExecutionPending {
		panic("instruction retired before callback")
	}
	p.calls++
}
