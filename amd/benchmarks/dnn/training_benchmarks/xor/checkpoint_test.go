package xor

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

func trainingSystem(t *testing.T) (*simulation.Simulation, *runner.Runner, *Benchmark, *vm.PageTable) {
	t.Helper()
	s := simulation.MakeBuilder().WithTaskTracking().Build()
	rt := emusystem.MakeBuilder().WithSimulation(s).Build()
	r := runner.New(s, rt)
	b := NewBenchmark(rt)
	b.Epoch = 3
	b.EnableTesting = true
	b.SelectGPU([]int{1})
	b.SetVerification(true)
	r.AddBenchmark(b)
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	for _, c := range s.Components() {
		if d, ok := c.(*driver.Comp); ok {
			return s, r, b, d.Resources.PageTable
		}
	}
	t.Fatal("Driver missing")
	return nil, nil, nil, nil
}
func finishTraining(t *testing.T, s *simulation.Simulation, r *runner.Runner, b *Benchmark, path string) {
	t.Helper()
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	state := b.trainer.State
	if !b.Ended || len(state.LastLoss) != 1 || math.IsNaN(state.LastLoss[0]) || state.LastLoss[0] <= 0 {
		t.Fatalf("invalid final training state: %+v", state.LastLoss)
	}
	for _, network := range state.Networks {
		if len(network.Operator.PendingFrees) != 0 {
			t.Fatal("deferred frees left at completion")
		}
	}
	if err := r.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(struct {
		Loss     []float64 `json:"loss"`
		Accuracy []float64 `json:"accuracy"`
		Time     int64     `json:"time"`
	}{state.LastLoss, state.LastAccuracy, int64(s.Engine().CurrentTime())})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".json", data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"three verified XOR training epochs and inference finished: loss %v, accuracy %v, time %d ps",
		state.LastLoss,
		state.LastAccuracy,
		s.Engine().CurrentTime(),
	)
}

//nolint:gocognit,funlen // Keep the fresh-process recovery scenario and assertions together.
func TestTrainingFreshProcessDeferredFree(t *testing.T) {
	if path := os.Getenv("MGPUSIM_XOR_RESTORE"); path != "" {
		s, r, b, _ := trainingSystem(t)
		if err := s.LoadCheckpoint(path); err != nil {
			t.Fatal(err)
		}
		finishTraining(t, s, r, b, path+".restored")
		return
	}
	path := filepath.Join(t.TempDir(), "pending-free.tar.gz")
	if !t.Run("uninterrupted", func(t *testing.T) {
		s, r, b, pt := trainingSystem(t)
		var pending []driver.Ptr
		for i := 0; i < 15; i++ {
			if _, err := r.Step(); err != nil {
				t.Fatal(err)
			}
			state := b.trainer.State
			if state.Phase == "forward" && len(state.Networks[0].Operator.PendingFrees) > 0 {
				pending = append(pending, state.Networks[0].Operator.PendingFrees...)
				break
			}
		}
		if len(pending) == 0 {
			t.Fatal("never reached a forward batch with pending frees")
		}
		for _, p := range pending {
			if _, ok := pt.Find(1, uint64(p)); !ok {
				t.Fatalf("allocation %x released before its batch completed", p)
			}
		}
		if err := r.SaveCheckpoint(path); err != nil {
			t.Fatal(err)
		}
		if _, err := r.Step(); err != nil {
			t.Fatal(err)
		}
		for _, p := range pending {
			if _, ok := pt.Find(1, uint64(p)); ok {
				t.Fatalf("allocation %x was not released after its batch completed", p)
			}
		}
		finishTraining(t, s, r, b, path+".uninterrupted")
	}) {
		return
	}
	debug.FreeOSMemory()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestTrainingFreshProcessDeferredFree$", "-test.v")
	cmd.Env = append(os.Environ(), "MGPUSIM_XOR_RESTORE="+path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("restored training: %v\n%s", err, output)
	} else {
		t.Logf("%s", output)
	}
	for _, suffix := range []string{"", ".json"} {
		want, err := os.ReadFile(path + ".uninterrupted" + suffix)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path + ".restored" + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(want, got) {
			t.Fatalf("fresh-process training changed final state %s", suffix)
		}
	}
}

func TestRepeatedSameProcessLoadRebindsCanonicalState(t *testing.T) {
	s, r, b, _ := trainingSystem(t)
	for i := 0; i < 5; i++ {
		if _, err := r.Step(); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "resume.tar.gz")
	if err := r.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	address := b.State
	finishTraining(t, s, r, b, path+".first")
	want, err := os.ReadFile(path + ".first")
	if err != nil {
		t.Fatal(err)
	}
	for replay := 0; replay < 2; replay++ {
		if err := s.LoadCheckpoint(path); err != nil {
			t.Fatal(err)
		}
		if b.State != address || b.Saved.State != address || b.trainer.State != &b.Trainer {
			t.Fatal("state/binding address diverged")
		}
		finishTraining(t, s, r, b, path+".replay")
		got, err := os.ReadFile(path + ".replay")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(want, got) {
			t.Fatal("same-process restore changed final simulator/trainer archive")
		}
	}
}
