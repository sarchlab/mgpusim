package cu

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

func TestSampledCompletionsKeepFutureWakeupsAndStableOrder(t *testing.T) {
	c, simulation := checkpointCUAndSimulation(t, false)
	group := lifecycleGroup(c, 4)
	waves := group.waves()
	due := []timing.VTimeInPicoSec{10001, 2000, 10001, 51001}
	for i, w := range waves {
		w.execution().State = wavefront.WfSampledWaiting
		c.scheduleSampledCompletion(due[i], w.handle)
	}
	want := []wavefront.Handle{waves[1].handle, waves[0].handle, waves[2].handle, waves[3].handle}
	for i, entry := range c.comp.State.SampledCompletions {
		if entry.Wave != want[i] {
			t.Fatal("completion queue is not stable and time ordered")
		}
	}
	port := newFakePort("CU.Top")
	c.ToACE = port
	engine := simulation.Engine().(*timing.SerialEngine)
	for _, step := range []struct {
		at        timing.VTimeInPicoSec
		remaining int
	}{{1999, 4}, {2000, 3}, {11000, 1}, {51000, 1}, {52000, 0}} {
		if err := engine.RunUntil(step.at); err != nil {
			t.Fatal(err)
		}
		if len(c.comp.State.SampledCompletions) != step.remaining {
			t.Fatalf("at %d: pending=%d want=%d", step.at, len(c.comp.State.SampledCompletions), step.remaining)
		}
	}
	if len(port.sent) != 1 || liveTestWaves(&c.comp.State) != 0 {
		t.Fatal("last future wakeup did not retire group exactly once")
	}
}

type scheduleDuringTick func() bool

func (f scheduleDuringTick) Run() bool { return f() }

func TestSampledCompletionAddedOnCurrentEdgeFinishesInSameTick(t *testing.T) {
	c, simulation := checkpointCUAndSimulation(t, false)
	wave := checkpointGroup(c).waves()[0]
	wave.execution().State = wavefront.WfSampledWaiting
	port := newFakePort("CU.Top")
	c.ToACE = port
	// Inject work after the first due drain, just as a zero-duration prediction
	// arriving through processInput does. TickAt cannot add another tick at 0.
	c.Scheduler = scheduleDuringTick(func() bool {
		c.scheduleSampledCompletion(c.CurrentTime(), wave.handle)
		return false
	})
	c.comp.TickAt(0)
	if err := simulation.Engine().(*timing.SerialEngine).RunUntil(0); err != nil {
		t.Fatal(err)
	}
	if len(port.sent) != 1 || len(c.comp.State.SampledCompletions) != 0 || liveTestWaves(&c.comp.State) != 0 {
		t.Fatal("current-edge completion waited for a later tick")
	}
}
