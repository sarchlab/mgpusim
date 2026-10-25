package runner

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

type stepBenchmark struct {
	benchmarks.Progress
	step func() error
}

func (b *stepBenchmark) Step() error                  { return b.step() }
func (*stepBenchmark) SelectGPU([]int)                {}
func (*stepBenchmark) SetUnifiedMemory()              {}
func (*stepBenchmark) SaveCheckpoint(io.Writer) error { return nil }
func (*stepBenchmark) LoadCheckpoint(io.Reader) error { return nil }

func newTestRunner(t *testing.T) (*Runner, *driver.Runtime) {
	t.Helper()
	s := sim.MakeBuilder().Build()
	rt := emusystem.MakeBuilder().WithSimulation(s).WithNumGPUs(1).Build()
	r := New(s, rt)
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	return r, rt
}
func initializeRunner(t *testing.T, r *Runner) {
	t.Helper()
	if err := r.simulation.Initialize(); err != nil {
		t.Fatal(err)
	}
}
func TestRunnerVisitsActiveBenchmarksInFixedRounds(t *testing.T) {
	r, _ := newTestRunner(t)
	var visits []int
	for i, rounds := range []int{2, 1, 3} {
		b := &stepBenchmark{}
		b.step = func() error {
			visits = append(visits, i)
			b.Phase++
			b.Ended = b.Phase == rounds
			if r.betweenRounds {
				t.Error("marked checkpoint-safe during Step")
			}
			if err := r.SaveCheckpoint(filepath.Join(t.TempDir(), "partial")); err == nil {
				t.Error("accepted checkpoint in partial round")
			}
			return nil
		}
		r.AddBenchmark(b)
	}
	initializeRunner(t, r)
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	want := []int{0, 1, 2, 0, 2, 2}
	if len(visits) != len(want) {
		t.Fatalf("visits %v", visits)
	}
	for i := range want {
		if visits[i] != want[i] {
			t.Fatalf("visits %v, want %v", visits, want)
		}
	}
	if !r.betweenRounds {
		t.Fatal("completed round is not checkpoint safe")
	}
	if err := r.SaveCheckpoint(filepath.Join(t.TempDir(), "complete")); err != nil {
		t.Fatal(err)
	}
}
func TestRunnerPropagatesStepFailureAndRetainsUnsafeBoundary(t *testing.T) {
	r, _ := newTestRunner(t)
	sentinel := errors.New("host input unavailable")
	r.AddBenchmark(&stepBenchmark{step: func() error { return sentinel }})
	initializeRunner(t, r)
	if _, err := r.Step(); !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "Benchmark[0]") {
		t.Fatalf("Step error %v", err)
	}
	if err := r.SaveCheckpoint(filepath.Join(t.TempDir(), "partial")); err == nil {
		t.Fatal("accepted checkpoint after partial failed round")
	}
}
func TestRunnerReportsUnscheduledQueueBeforeHostStep(t *testing.T) {
	r, rt := newTestRunner(t)
	ctx := rt.Init()
	rt.SelectGPU(ctx, 1)
	q := rt.CreateCommandQueue(ctx)
	rt.EnqueueRecordEvent(q)
	visited := false
	r.AddBenchmark(&stepBenchmark{step: func() error { visited = true; return nil }})
	initializeRunner(t, r)
	if _, err := r.Step(); err == nil || !strings.Contains(err.Error(), "queue 1 stalled with 1 unfinished") {
		t.Fatalf("stall error %v", err)
	}
	if visited {
		t.Fatal("host Step ran despite unfinished command")
	}
}
