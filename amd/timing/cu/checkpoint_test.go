package cu

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

func TestCheckpointPreservesBarrierAndLDSIdentity(t *testing.T) {
	original := checkpointCU(t)
	grid := kernels.NewGridBuilder(original.comp)
	grid.SetKernel(kernels.KernelLaunchInfo{
		CodeObject: &insts.KernelCodeObject{KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{}},
		Packet: &kernels.HsaKernelDispatchPacket{GridSizeX: 128, GridSizeY: 1, GridSizeZ: 1,
			WorkgroupSizeX: 128, WorkgroupSizeY: 1, WorkgroupSizeZ: 1},
	})
	raw := grid.NextWG()
	wg := original.wrapWG(raw, messaging.Msg{ID: original.comp.NewID(), Payload: protocol.MapWGReq{}})
	g := original.comp.State.Groups[wg.slot]
	g.LDS = []byte{11, 13, 17, 19}
	original.comp.State.Groups[wg.slot] = g
	for _, wf := range wg.waves() {
		wf.execution().State = wavefront.WfAtBarrier
		wf.execution().SampleIssueTime = 123
		wf.execution().SampleFinishTime = 456
		original.WfPools[0].AddWf(wf)
	}
	original.comp.State.Scheduler.Barrier = []uint64{wfID(wg.waves()[0])}
	original.comp.State.Scheduler.NoProgress = 27
	original.comp.State.LDS.Exec = wfID(wg.waves()[1])
	original.comp.State.LDS.Cycles = 3
	restored := roundTripCUState(t, original)
	restoredWG := &WorkGroup{cu: restored, UID: wg.UID}
	if restoredWG == wg || !bytes.Equal(restoredWG.state().LDS, wg.state().LDS) {
		t.Fatal("LDS not restored independently")
	}
	rs := restored.Scheduler.(*SchedulerImpl)
	if restored.wave(rs.state().Barrier[0]).state() != restoredWG.waves()[0].state() ||
		restored.WfPools[0].waves()[0].state() != restoredWG.waves()[0].state() {
		t.Fatal("barrier and pool do not share the restored wavefront")
	}
	lds := restored.LDSUnit.(*LDSUnit)
	if restored.wave(lds.state().Exec).state() != restoredWG.waves()[1].state() || lds.state().Cycles != 3 ||
		rs.state().NoProgress != 27 {
		t.Fatal("pipeline progress was not restored")
	}
	if restoredWG.waves()[0].execution().SampleIssueTime != 123 ||
		restoredWG.waves()[0].execution().SampleFinishTime != 456 {
		t.Fatal("sampling timestamps lost")
	}
	if &restoredWG.waves()[0].group().state().LDS[0] != &restoredWG.waves()[1].group().state().LDS[0] {
		t.Fatal("restored waves do not resolve shared canonical LDS")
	}
	restoredWG.state().LDS[0] = 23
	if wg.state().LDS[0] != 11 {
		t.Fatal("restored LDS still aliases source")
	}
}

func checkpointCU(t *testing.T, specs ...Spec) *ComputeUnit {
	t.Helper()
	return checkpointCUWithTracking(t, false, specs...)
}

func checkpointCUWithTracking(t *testing.T, tracking bool, specs ...Spec) *ComputeUnit {
	t.Helper()
	builder := sim.MakeBuilder()
	if tracking {
		builder = builder.WithTaskTracking()
	}
	simulation := builder.Build()
	t.Cleanup(func() {
		if err := simulation.Terminate(); err != nil {
			t.Error(err)
		}
	})
	spec := DefaultSpec()
	if len(specs) > 0 {
		spec = specs[0]
	}
	comp := MakeBuilder().WithSimulation(simulation).WithSpec(spec).Build("CU")
	for _, name := range []string{DispatchPortName, InstMemPortName, ScalarMemPortName, VectorMemPortName} {
		comp.BindPort(name, twowaybuffered.MakeBuilder().WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).Build())
	}
	simulation.Initialize()
	return MiddlewareOf(comp)
}

func checkpointGroup(c *ComputeUnit) *WorkGroup {
	grid := kernels.NewGridBuilder(c.comp)
	grid.SetKernel(kernels.KernelLaunchInfo{
		CodeObject: &insts.KernelCodeObject{KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{}},
		Packet: &kernels.HsaKernelDispatchPacket{
			GridSizeX: 64, GridSizeY: 1, GridSizeZ: 1, WorkgroupSizeX: 64, WorkgroupSizeY: 1, WorkgroupSizeZ: 1,
		},
	})
	return c.wrapWG(grid.NextWG(), messaging.Msg{ID: c.comp.NewID(), Payload: protocol.MapWGReq{}})
}

func checkpointInstruction(t *testing.T, c *ComputeUnit) *wavefront.Inst {
	t.Helper()
	raw := []byte{0, 0, 0x80, 0xbf} // s_nop 0
	decoded, err := c.Decoder.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	i := wavefront.NewInst(c.comp.NewID(), decoded)
	i.Raw = raw
	return i
}

func TestCheckpointRegistersAndSIMDResumeFromCanonicalState(t *testing.T) {
	original := checkpointCU(t)
	wg := checkpointGroup(original)
	wf := wg.waves()[0]
	wf.SetDynamicInst(checkpointInstruction(t, original))
	wf.execution().InstBuffer = make([]byte, 64)
	wf.execution().State = wavefront.WfRunning
	wf.execution().InFlightInsts = 1
	original.comp.State.InstTasks[wf.execution().InstID] = 0
	original.comp.State.SIMDs[0] = SIMDState{Exec: wfID(wf), Cycles: 2}
	original.comp.State.SRegisters[8] = 37
	original.comp.State.VRegisters[0][12] = 41
	var archive bytes.Buffer
	if err := original.comp.SaveCheckpoint(&archive); err != nil {
		t.Fatal(err)
	}
	restored := checkpointCU(t)
	scalar, vector := restored.SRegFile, restored.VRegFile[0]
	if err := restored.comp.LoadCheckpoint(&archive); err != nil {
		t.Fatal(err)
	}
	// Previously acquired register wrappers must resolve the replaced State
	// before any MiddlewareOf call or simulated event.
	data := make([]byte, 4)
	scalar.Read(RegisterAccess{Reg: insts.SReg(2), Data: data})
	if data[0] != 37 {
		t.Fatalf("stale scalar storage: %v", data)
	}
	vector.Read(RegisterAccess{Reg: insts.VReg(3), Data: data})
	if data[0] != 41 {
		t.Fatalf("stale vector storage: %v", data)
	}
	unit := restored.SIMDUnit[0].(*SIMDUnit)
	unit.alu = &mockALU{}
	unit.Run()
	if restored.comp.State.SIMDs[0].Cycles != 1 {
		t.Fatal("SIMD did not resume its saved cycle")
	}
	unit.Run()
	view := restored.wave(wfID(wf))
	if view.PC() != 4 || view.execution().State != wavefront.WfReady || restored.comp.State.SIMDs[0].Exec != 0 {
		t.Fatal("SIMD completion did not update canonical wave")
	}
	if restored.comp.State.Waves[slotIndex(wg.state().Waves[0])].Execution.PC != 4 {
		t.Fatal("wave view is a shadow copy")
	}
}

func TestInstructionRegistryBoundedWithinLongLivedGroup(t *testing.T) {
	c := checkpointCU(t)
	wf := checkpointGroup(c).waves()[0]
	for i := 0; i < 2000; i++ {
		next := checkpointInstruction(t, c)
		wf.SetNextInst(next)
		wf.SetDynamicInst(wf.NextInst())
		wf.SetNextInst(nil)
		if len(c.comp.State.Instructions) != 1 {
			t.Fatalf("instruction registry grows at iteration %d", i)
		}
	}
}

func TestCheckpointRejectsDifferentMemoryDestinations(t *testing.T) {
	c := checkpointCU(t)
	var archive bytes.Buffer
	if err := c.comp.SaveCheckpoint(&archive); err != nil {
		t.Fatal(err)
	}
	c.comp.Spec.InstMem = "DifferentROB.Top"
	if err := c.comp.LoadCheckpoint(&archive); err == nil {
		t.Fatal("changed immutable destination was accepted")
	}
}

func TestCheckpointScoreboardAndPipelinedSIMDIdentity(t *testing.T) {
	spec := DefaultSpec()
	spec.RegisterScoreboard = true
	original := checkpointCU(t, spec)
	wf := checkpointGroup(original).waves()[0]
	wf.SetDynamicInst(checkpointInstruction(t, original))
	wf.execution().InstBuffer = make([]byte, 64)
	wf.execution().State = wavefront.WfRunning
	wf.execution().InFlightInsts = 1
	scoreboard := wf.scoreboard()
	scoreboard.VGPRBusyUntil[7] = 9
	original.comp.State.InstTasks[wf.execution().InstID] = 0
	original.comp.State.SIMDs[0].Slots = []SIMDSlotState{{WF: wfID(wf), Cycles: 2}}
	var archive bytes.Buffer
	if err := original.comp.SaveCheckpoint(&archive); err != nil {
		t.Fatal(err)
	}
	restored := checkpointCU(t, spec)
	if err := restored.comp.LoadCheckpoint(&archive); err != nil {
		t.Fatal(err)
	}
	restored = MiddlewareOf(restored.comp)
	view := restored.wave(wfID(wf))
	canonical := restored.comp.State.Groups[wf.group().slot]
	if view.scoreboard() != &restored.comp.State.Waves[slotIndex(canonical.Waves[0])].Scoreboard {
		t.Fatal("scoreboard is not canonical")
	}
	if view.scoreboard().VGPRBusyUntil[7] != 9 {
		t.Fatal("scoreboard progress was lost")
	}
	unit := restored.SIMDUnit[0].(*SIMDUnit)
	unit.alu = &mockALU{}
	unit.Run()
	if restored.comp.State.SIMDs[0].Slots[0].Cycles != 1 {
		t.Fatal("pipeline slot latency lost")
	}
	unit.Run()
	if len(restored.comp.State.SIMDs[0].Slots) != 0 || view.PC() != 4 {
		t.Fatal("pipeline slot did not complete")
	}
}

func TestCompletedGroupRetiresAfterFetchAheadResponse(t *testing.T) {
	c := checkpointCU(t)
	wg := checkpointGroup(c)
	wf := wg.waves()[0]
	wf.SetDynamicInst(checkpointInstruction(t, c))
	c.WfPools[0].AddWf(wf)
	req := messaging.Msg{ID: c.comp.NewID(), Payload: memprotocol.ReadReq{AccessByteSize: 4}}
	c.comp.State.InstFetch = []FetchState{{Req: req, WF: wfID(wf)}}
	wf.execution().State = wavefront.WfCompleted
	port := newFakePort("CU.ACE")
	c.ToACE = port
	c.retireGroups()
	if c.wave(wfID(wf)).state() != wf.state() || liveTestGroups(&c.comp.State) != 1 {
		t.Fatal("fetch-ahead wave retired early")
	}
	if len(port.sent) != 0 || c.WfPools[0].Availability() != c.comp.Spec.WfPoolSize-1 {
		t.Fatal("CP resources released before late fetch")
	}
	c.handleFetchReturn(messaging.Msg{RspTo: req.ID, Payload: memprotocol.DataReadyRsp{Data: []byte{0, 0, 0, 0}}})
	c.retireGroups()
	if liveTestGroups(&c.comp.State) != 0 || liveTestWaves(&c.comp.State) != 0 || len(c.comp.State.Instructions) != 0 {
		t.Fatal("completed graph was retained")
	}
}

func roundTripCUState(t *testing.T, original *ComputeUnit) *ComputeUnit {
	t.Helper()
	var archive bytes.Buffer
	if err := original.comp.SaveCheckpoint(&archive); err != nil {
		t.Fatal(err)
	}
	restored := checkpointCU(t, original.comp.Spec)
	if err := restored.comp.LoadCheckpoint(&archive); err != nil {
		t.Fatal(err)
	}
	return MiddlewareOf(restored.comp)
}

func TestSampledCompletionRetiresGroupWithoutAnotherTick(t *testing.T) {
	c := checkpointCU(t)
	wg := checkpointGroup(c)
	wf := wg.waves()[0]
	wf.execution().State = wavefront.WfSampledCompleted
	wf.SetDynamicInst(checkpointInstruction(t, c))
	c.WfPools[0].AddWf(wf)
	c.comp.State.WFTimes[wf.UID] = 100
	c.comp.State.WFTasks[wf.UID] = 0
	c.comp.State.WGTasks[wg.state().Request.ID] = 0
	port := newFakePort("CU.Top")
	c.ToACE = port
	c.Handle(wavefront.NewWfCompletionEvent(c.comp.NewID(), 200, c.wfCompletionHandlerID, wfID(wf)))
	if len(port.sent) != 1 {
		t.Fatal("sampled completion response was not sent")
	}
	if liveTestGroups(&c.comp.State) != 0 || liveTestWaves(&c.comp.State) != 0 ||
		len(c.comp.State.Instructions) != 0 {
		t.Fatal("sampled completion retained its canonical execution graph")
	}
	if len(c.comp.State.WFTimes) != 0 || len(c.comp.State.WFTasks) != 0 || len(c.comp.State.WGTasks) != 0 {
		t.Fatal("sampled completion retained timing or task state")
	}
}

func TestWavefrontLookupUsesCurrentStateAfterLoad(t *testing.T) {
	original := checkpointCU(t)
	wf := checkpointGroup(original).waves()[0]
	var archive bytes.Buffer
	if err := original.comp.SaveCheckpoint(&archive); err != nil {
		t.Fatal(err)
	}
	restored := checkpointCU(t)
	if err := restored.comp.LoadCheckpoint(&archive); err != nil {
		t.Fatal(err)
	}
	// Lookup resolves current State; neither Tick nor MiddlewareOf runs after load.
	got := restored.wavefrontByID(wf.UID)
	if got == wf || got.state() != restored.findWavefront(wf.UID).state() ||
		got.state() != restored.wave(wfID(wf)).state() {
		t.Fatal("wavefront lookup does not share the canonical restored view")
	}
	for _, id := range []uint64{0, ^uint64(0)} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("unknown wavefront %d did not panic", id)
				}
			}()
			restored.wavefrontByID(id)
		}()
	}
}
