package gputraining

import (
	"bytes"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training/optimization"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

type trainingWorkload struct {
	benchmarks.Progress
	runtime *driver.Runtime
	trainer DataParallelismMultiGPUTrainer
}

func (*trainingWorkload) SelectGPU([]int)   {}
func (*trainingWorkload) SetUnifiedMemory() {}
func (b *trainingWorkload) Step() error {
	if b.Phase == 0 {
		b.initialize()
		b.Phase = 1
	}
	if err := b.trainer.Step(); err != nil {
		return err
	}
	b.Ended = b.trainer.IsEnded()
	return nil
}
func (b *trainingWorkload) initialize() {
	t := &b.trainer
	t.Driver = b.runtime
	t.Epoch = 2
	t.BatchSize = 1
	t.MaxBatchPerEpoch = 1
	t.GPUs = []int{1, 2}
	base := b.runtime.Init()
	for gpu := 1; gpu <= 2; gpu++ {
		ctx := b.runtime.InitWithExistingPID(base)
		b.runtime.SelectGPU(ctx, gpu)
		o := gputensor.NewGPUOperator(b.runtime, ctx)
		o.SetVerification(true)
		n := training.Network{Layers: []layers.Layer{
			layers.NewConv2D(0, o, []int{1, 4, 4}, []int{2, 1, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(o),
			layers.NewMaxPoolingLayer(o, []int{2, 2}, []int{0, 0}, []int{2, 2}),
			layers.NewFullyConnectedLayer(3, o, 8, 2),
		}}
		data := make([]float64, 16)
		for j := range data {
			data[j] = float64(j+gpu) / 17
		}
		source := &training.ResidentDataSource{
			Operator: o,
			State:    training.DataState{Data: data, Labels: []int{gpu - 1}, Shape: []int{1, 4, 4}, Descriptor: "NCHW"},
		}
		t.TensorOperators = append(t.TensorOperators, o)
		t.Contexts = append(t.Contexts, ctx)
		t.Networks = append(t.Networks, n)
		t.DataSource = append(t.DataSource, source)
		t.LossFunc = append(t.LossFunc, training.NewSoftmaxCrossEntropy(o))
		t.OptimizationAlg = append(t.OptimizationAlg, optimization.NewAdam(o, 0.01))
	}
}

type workloadState struct {
	Progress benchmarks.Progress
	Trainer  State
}

func (b *trainingWorkload) SaveCheckpoint(w io.Writer) error {
	s, err := b.trainer.Snapshot()
	if err != nil {
		return err
	}
	return modeling.WriteCheckpoint(w, "tiny-conv-training-v1", workloadState{b.Progress, s}, nil)
}
func (b *trainingWorkload) LoadCheckpoint(r io.Reader) error {
	var s workloadState
	if err := modeling.ReadCheckpoint(r, "tiny-conv-training-v1", &s, nil); err != nil {
		return err
	}
	t, err := Restore(b.runtime, s.Trainer)
	if err != nil {
		return err
	}
	b.Progress = s.Progress
	b.trainer = t
	return nil
}
func collectiveSystem(t *testing.T) (*sim.Simulation, *runner.Runner, *trainingWorkload) {
	t.Helper()
	s := sim.MakeBuilder().WithTaskTracking().Build()
	rt := emusystem.MakeBuilder().WithSimulation(s).WithNumGPUs(2).Build()
	r := runner.New(s, rt)
	b := &trainingWorkload{runtime: rt}
	r.AddBenchmark(b)
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	return s, r, b
}
func finishCollectiveTraining(t *testing.T, s *sim.Simulation, r *runner.Runner, b *trainingWorkload, path string) {
	t.Helper()
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	state, err := b.trainer.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.LastLoss) != 2 {
		t.Fatal("missing GPU losses")
	}
	for i, loss := range state.LastLoss {
		if math.IsNaN(loss) || loss <= 0 {
			t.Fatalf("GPU %d loss %g", i, loss)
		}
		if len(state.Networks[i].Operator.PendingFrees) != 0 {
			t.Fatal("deferred frees remain")
		}
	}
	if err := r.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	t.Logf("two GPU conv/relu/maxpool/FC training: losses %v, time %d ps", state.LastLoss, s.Engine().CurrentTime())
}

//nolint:funlen,gocognit // Keep the fresh-process collective recovery scenario together.
func TestMultiGPUTrainingFreshProcessCollective(t *testing.T) {
	if path := os.Getenv("MGPUSIM_COLLECTIVE_RESTORE"); path != "" {
		s, r, b := collectiveSystem(t)
		if err := s.LoadCheckpoint(path); err != nil {
			t.Fatal(err)
		}
		finishCollectiveTraining(t, s, r, b, path+".restored")
		return
	}
	path := filepath.Join(t.TempDir(), "collective.tar.gz")
	if !t.Run("uninterrupted", func(t *testing.T) {
		s, r, b := collectiveSystem(t)
		found := false
		for i := 0; i < 80; i++ {
			if _, err := r.Step(); err != nil {
				t.Fatal(err)
			}
			c := b.trainer.collective
			if b.trainer.phase == "reduce" && c != nil && c.Stage == 1 {
				if err := r.SaveCheckpoint(path); err != nil {
					t.Fatal(err)
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatal("did not reach submitted reduction stage")
		}
		finishCollectiveTraining(t, s, r, b, path+".uninterrupted")
	}) {
		return
	}
	cmd := exec.CommandContext(
		t.Context(),
		os.Args[0],
		"-test.run=^TestMultiGPUTrainingFreshProcessCollective$",
		"-test.v",
	)
	cmd.Env = append(os.Environ(), "MGPUSIM_COLLECTIVE_RESTORE="+path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("restore: %v\n%s", err, output)
	} else {
		t.Logf("%s", output)
	}
	want, err := os.ReadFile(path + ".uninterrupted")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path + ".restored")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Fatal("fresh-process collective recovery changed final simulator or training state")
	}
}
