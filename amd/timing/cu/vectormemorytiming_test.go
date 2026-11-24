package cu

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// vmuTickRecorder replaces the scheduler, so the only wakeups are those the
// vector memory unit asks for.
type vmuTickRecorder struct {
	c      *ComputeUnit
	cycles []uint64
}

func (r *vmuTickRecorder) Run() bool {
	r.cycles = append(r.cycles, r.c.comp.Spec.Freq.Cycle(r.c.CurrentTime()))
	return false
}

type vmuSend struct{ ID, Cycle uint64 }

type vmuTimedPort struct {
	*fakePort
	c     *ComputeUnit
	sends []vmuSend
}

func (p *vmuTimedPort) Send(msg messaging.Msg) {
	p.fakePort.Send(msg)
	p.sends = append(p.sends, vmuSend{msg.ID, p.c.comp.Spec.Freq.Cycle(p.c.CurrentTime())})
}

type vmuFixture struct {
	t          *testing.T
	c          *ComputeUnit
	simulation *sim.Simulation
	engine     *timing.SerialEngine
	unit       *VectorMemoryUnit
	port       *vmuTimedPort
	ticks      *vmuTickRecorder
}

func newVMUFixture(t *testing.T, spec Spec) *vmuFixture {
	t.Helper()
	c, simulation := checkpointCUAndSimulation(t, false, spec)
	f := &vmuFixture{
		t: t, c: c, simulation: simulation, engine: simulation.Engine().(*timing.SerialEngine),
		unit: c.VectorMemUnit.(*VectorMemoryUnit), ticks: &vmuTickRecorder{c: c},
		port: &vmuTimedPort{fakePort: newFakePort("CU.VectorMem"), c: c},
	}
	c.Scheduler = f.ticks
	c.ToVectorMem = f.port
	return f
}

func (f *vmuFixture) run() {
	f.t.Helper()
	if err := f.engine.Run(); err != nil {
		f.t.Fatal(err)
	}
}

func (f *vmuFixture) runUntilCycle(cycle uint64) {
	f.t.Helper()
	if err := f.engine.RunUntil(timing.VTimeInPicoSec(cycle) * f.c.comp.Spec.Freq.Period()); err != nil {
		f.t.Fatal(err)
	}
}

// flatWave returns a wave running a FLAT load that coalesces into one read.
func (f *vmuFixture) flatWave() *Wavefront {
	f.t.Helper()
	w := checkpointGroup(f.c).waves()[0]
	w.setTestInst(lifecycleInstruction(f.t, f.c, []byte{0, 0, 0x50, 0xdc, 0, 0, 0, 0}))
	w.execution().State = wavefront.WfRunning
	w.execution().InFlightInsts = 1
	f.useCoalescedRead(w, f.c.comp.NewID())
	return w
}

func (f *vmuFixture) useCoalescedRead(w *Wavefront, id uint64) {
	f.unit.coalescer = &fakeCoalescer{toReturn: []VectorMemAccessInfo{{
		Read:      messaging.Msg{ID: id, Payload: memprotocol.ReadReq{AccessByteSize: 64}},
		Wavefront: w,
		InstID:    w.InstID(),
	}}}
}

// queueRead admits a read that coalesced the given number of lanes.
func (f *vmuFixture) queueRead(lanes int) uint64 {
	req := messaging.Msg{ID: f.c.comp.NewID(), Src: "CU.VectorMem", Dst: "VectorMem",
		Payload: memprotocol.ReadReq{AccessByteSize: 64}}
	access := VectorAccessState{Read: req, Lanes: make([]LaneState, lanes)}
	f.c.admitVector(access)
	f.unit.state().Waiting = append(f.unit.state().Waiting, access)
	return req.ID
}

func vmuTimingSpec(instStages, transStages, width, penalty int) Spec {
	spec := DefaultSpec()
	spec.VecMemInstPipelineStages = instStages
	spec.VecMemTransPipelineStages = transStages
	spec.VecMemTransPipelineWidth = width
	spec.MaxCoalescingPenalty = penalty
	return spec
}

func TestVectorMemorySleepsBetweenPipelineEdges(t *testing.T) {
	for _, tc := range []struct {
		name          string
		inst, trans   uint64
		width, buffer int
	}{
		{name: "gcn3", inst: 6, trans: 10, width: 1, buffer: 8},
		{name: "cdna3", inst: 2, trans: 4, width: 8, buffer: 64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := vmuTimingSpec(int(tc.inst), int(tc.trans), tc.width, 0)
			spec.MemPipelineBufferSize = tc.buffer
			f := newVMUFixture(t, spec)
			w := f.flatWave()
			id := f.unit.coalescer.(*fakeCoalescer).toReturn[0].Read.ID
			f.unit.AcceptWave(w)
			f.run()

			// Drain, execute, enter the transaction pipeline, then one idle
			// tick; sleep; drain, send, then one idle tick.
			enter := tc.inst + 2
			want := []uint64{tc.inst, tc.inst + 1, enter, enter + 1, enter + tc.trans, enter + tc.trans + 1,
				enter + tc.trans + 2}
			if !slices.Equal(f.ticks.cycles, want) {
				t.Fatalf("ticks at %v, want %v", f.ticks.cycles, want)
			}
			if sends := []vmuSend{{id, enter + tc.trans + 1}}; !slices.Equal(f.port.sends, sends) {
				t.Fatalf("sends %v, want %v", f.port.sends, sends)
			}
		})
	}
}

func TestCoalescingPenaltyCountsWithoutOtherProgress(t *testing.T) {
	// One of 16 lanes wastes 15/16 of a penalty of 32, which is 30 cycles and
	// outlasts the two-stage pipeline, so nothing else keeps the CU awake.
	f := newVMUFixture(t, vmuTimingSpec(2, 2, 8, 32))
	first, second := f.queueRead(1), f.queueRead(16)
	f.c.comp.TickLater()
	f.run()

	want := []vmuSend{{first, 4}, {second, 35}}
	if !slices.Equal(f.port.sends, want) {
		t.Fatalf("sends %v, want %v", f.port.sends, want)
	}
	if f.unit.state().Stall != 0 || len(f.unit.state().Waiting) != 0 {
		t.Fatal("coalescing penalty stranded a request")
	}
}

func TestExpiredCoalescingPenaltyDoesNotDelayLaterInput(t *testing.T) {
	f := newVMUFixture(t, vmuTimingSpec(2, 2, 8, 32))
	f.queueRead(1)
	f.c.comp.TickLater()
	f.run()
	if last := f.ticks.cycles[len(f.ticks.cycles)-1]; last != 31 || f.unit.state().Stall != 0 {
		t.Fatalf("penalty countdown ended at cycle %d with %d left", last, f.unit.state().Stall)
	}

	// The next read enters on the next edge and pays no leftover penalty.
	later := f.queueRead(16)
	f.c.comp.TickLater()
	f.run()
	if got := f.port.sends[len(f.port.sends)-1]; got != (vmuSend{later, 35}) {
		t.Fatalf("later read sent as %v", got)
	}
}

func TestStalledOutputResumesInOrderOnPortFree(t *testing.T) {
	f := newVMUFixture(t, vmuTimingSpec(2, 2, 1, 0))
	f.port.full = true
	var ids []uint64
	for range 12 {
		ids = append(ids, f.queueRead(16))
	}
	f.c.comp.TickLater()
	f.run()

	// The eight-entry output and two-stage lane hold ten; two stay queued. The
	// engine has drained, so a blocked CU is not spinning on the full port.
	state := f.unit.state()
	if state.TransactionsOutput.Size() != 8 || state.TransactionsPipeline.Size() != 2 || len(state.Waiting) != 2 {
		t.Fatalf("output %d, pipeline %d, waiting %d", state.TransactionsOutput.Size(),
			state.TransactionsPipeline.Size(), len(state.Waiting))
	}

	blockedAt := f.c.comp.Spec.Freq.Cycle(f.c.CurrentTime())
	f.port.full = false
	f.c.comp.NotifyPortFree(f.port)
	f.run()

	got := make([]uint64, 0, len(f.port.sends))
	for _, send := range f.port.sends {
		got = append(got, send.ID)
	}
	if !slices.Equal(got, ids) {
		t.Fatalf("sent %v, want %v", got, ids)
	}
	if f.port.sends[0].Cycle != blockedAt+1 {
		t.Fatalf("first send at cycle %d, want %d", f.port.sends[0].Cycle, blockedAt+1)
	}
	if state.TransactionsOutput.Size() != 0 || state.TransactionsPipeline.Size() != 0 || len(state.Waiting) != 0 {
		t.Fatal("a transaction was stranded after the port freed")
	}
}

func TestRepeatedAdmissionsAtOneEdgeShareOneWake(t *testing.T) {
	engine := newFakeEngine()
	c := newTestComputeUnit("CU", engine)
	unit := NewVectorMemoryUnit(c, &fakeCoalescer{})
	engine.now = 5000

	var waves []vectorMemInst
	for range c.comp.Spec.SIMDCount {
		w := newTestWave()
		registerTestWave(c, w)
		if !unit.CanAcceptWave() {
			t.Fatal("a free lane rejected a wave")
		}
		unit.AcceptWave(w)
		waves = append(waves, vectorMemInst{Handle: handleOf(w)})
	}
	if unit.CanAcceptWave() || unit.CanAcceptWave() {
		t.Fatal("stage 0 accepted more than one item per lane in a cycle")
	}
	if len(engine.scheduled) != 1 || engine.scheduled[0].Time() != 11000 {
		t.Fatalf("scheduled %d wakes, want one at 11000 ps", len(engine.scheduled))
	}

	engine.now = 6000
	if !unit.CanAcceptWave() {
		t.Fatal("stage 0 did not free on the next edge")
	}
	engine.now = 10000
	unit.Run()
	if unit.state().InstructionsOutput.Size() != 0 {
		t.Fatal("instruction left the pipeline before its latency")
	}
	engine.now = 11000
	unit.Run()
	for _, want := range waves {
		if got, ok := unit.state().InstructionsOutput.Pop(); !ok || got != want {
			t.Fatalf("output %v, want %v", got, want)
		}
	}
}

func TestCheckpointRestoresVectorPipelinesInFlight(t *testing.T) {
	spec := vmuTimingSpec(6, 10, 1, 16)
	f := newVMUFixture(t, spec)
	// Admission requires increasing IDs, so the queued reads take theirs first.
	f.queueRead(1)
	f.queueRead(16)
	w := f.flatWave()
	flatID := f.unit.coalescer.(*fakeCoalescer).toReturn[0].Read.ID
	f.unit.AcceptWave(w)
	f.c.comp.TickLater()
	f.runUntilCycle(3)

	state := f.unit.state()
	if state.InstructionsPipeline.Size() != 1 || state.TransactionsPipeline.Size() != 1 ||
		state.Stall == 0 || len(state.Waiting) != 1 {
		t.Fatalf("checkpoint missed in-flight work: %d instructions, %d transactions, stall %d, %d waiting",
			state.InstructionsPipeline.Size(), state.TransactionsPipeline.Size(), state.Stall, len(state.Waiting))
	}
	instructions, transactions := state.InstructionsPipeline.Stages(), state.TransactionsPipeline.Stages()
	path := filepath.Join(t.TempDir(), "vector-pipelines.tar.gz")
	if err := f.simulation.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	ticksAtCheckpoint := len(f.ticks.cycles)
	f.run()

	restored := newVMUFixture(t, spec)
	if err := restored.simulation.LoadCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	restoredState := restored.unit.state()
	if !slices.Equal(restoredState.InstructionsPipeline.Stages(), instructions) ||
		!slices.Equal(restoredState.TransactionsPipeline.Stages(), transactions) {
		t.Fatal("restored pipelines moved their in-flight items")
	}
	restored.useCoalescedRead(restored.c.wave(handleOf(w)), flatID)
	restored.run()

	if len(f.port.sends) != 3 || !slices.Equal(restored.port.sends, f.port.sends) {
		t.Fatalf("restored sends %v, uninterrupted %v", restored.port.sends, f.port.sends)
	}
	if want := f.ticks.cycles[ticksAtCheckpoint:]; !slices.Equal(restored.ticks.cycles, want) {
		t.Fatalf("restored ticks %v, want uninterrupted suffix %v", restored.ticks.cycles, want)
	}
}
