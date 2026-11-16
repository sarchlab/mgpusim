package gputraining

import (
	"bytes"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training/optimization"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

type trainingWorkload struct {
	benchmarks.Progress
	runtime *gpuruntime.Runtime
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
	b.trainer.State = &State{}
	t := &b.trainer
	t.Runtime = b.runtime
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
			State:    &training.DataState{Data: data, Labels: []int{gpu - 1}, Shape: []int{1, 4, 4}, Descriptor: "NCHW"},
		}
		t.TensorOperators = append(t.TensorOperators, o)
		t.Contexts = append(t.Contexts, ctx)
		t.Networks = append(t.Networks, n)
		t.DataSource = append(t.DataSource, source)
		t.LossFunc = append(t.LossFunc, training.NewSoftmaxCrossEntropy(o))
		t.OptimizationAlg = append(t.OptimizationAlg, optimization.NewAdam(o, 0.01))
	}
	if err := t.Connect(); err != nil {
		panic(err)
	}
}

type workloadState struct {
	Progress benchmarks.Progress
	Trainer  *State
}

func (b *trainingWorkload) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, workloadState{b.Progress, b.trainer.State}, nil)
}
func (b *trainingWorkload) LoadCheckpoint(r io.Reader) error {
	var s workloadState
	if err := modeling.ReadCheckpoint(r, &s, nil); err != nil {
		return err
	}
	t, err := Bind(b.runtime, s.Trainer)
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
	checkReplicatedParameters(t, s, b)
	state := b.trainer.State
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

// The two replicas see different input batches, but reduced gradients and Adam
// must leave every trainable parameter identical after the final update.
func checkReplicatedParameters(t *testing.T, s *sim.Simulation, b *trainingWorkload) {
	t.Helper()
	trainer := &b.trainer
	ids := make([][]gpuruntime.HostBuffer, len(trainer.Networks))
	for gpu, network := range trainer.Networks {
		ids[gpu] = make([]gpuruntime.HostBuffer, len(network.Layers))
		for i, layer := range network.Layers {
			if p := layer.Parameters(); p != nil {
				ids[gpu][i] = trainer.TensorOperators[gpu].EnqueueReadback(p)
			}
		}
	}
	for _, c := range s.Components() {
		if c.Name() == "Driver" {
			c.(driver.Ticker).TickLater()
		}
	}
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for i, layer := range trainer.Networks[0].Layers {
		p := layer.Parameters()
		if p == nil {
			continue
		}
		other := trainer.Networks[1].Layers[i].Parameters()
		if err := trainer.TensorOperators[0].ReadTensor(ids[0][i], p); err != nil {
			t.Fatal(err)
		}
		if err := trainer.TensorOperators[1].ReadTensor(ids[1][i], other); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(p.Vector(), other.Vector()) {
			t.Fatalf("layer %d parameters differ after reduction and Adam", i)
		}
		checked += p.NumElement()
	}
	if checked == 0 {
		t.Fatal("no trainable parameters checked")
	}
	t.Logf("checked %d replicated parameters", checked)
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
			c := b.trainer.Collective
			if b.trainer.Phase == "reduce" && c != nil && c.Stage == 1 {
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

//nolint:funlen // Keep the shared identity and readback lifecycle in one regression.
func TestCrossEntropySharedTensorIdentitySurvivesBinding(t *testing.T) {
	s := sim.MakeBuilder().Build()
	rt := emusystem.MakeBuilder().WithSimulation(s).WithNumGPUs(1).Build()
	o := gputensor.NewGPUOperator(rt, rt.Init())
	v := o.CreateWithData([]float64{0.25, 0.75}, []int{1, 2}, "")
	trainer := DataParallelismMultiGPUTrainer{
		State: &State{
			Phase: "lossRead",
			Values: []tensor.ID{
				o.Reference(v),
			},
			LossInputs: []tensor.ID{
				o.Reference(v),
			},
			Labels: [][]int{
				{
					1,
				},
			},
			LossReadbacks: []gpuruntime.HostBuffer{
				o.EnqueueReadback(v),
			},
		},
		Runtime: rt,

		TensorOperators: []*gputensor.GPUOperator{o}, Networks: []training.Network{{}},
		DataSource:      []training.DataSource{&training.ResidentDataSource{Operator: o, State: &training.DataState{}}},
		LossFunc:        []training.LossFunction{training.NewCrossEntropy(o)},
		OptimizationAlg: []optimization.Alg{optimization.NewSGD(o, 0.01)},
	}
	if err := trainer.Connect(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := modeling.WriteCheckpoint(&buf, trainer.State, nil); err != nil {
		t.Fatal(err)
	}
	var state State
	if err := modeling.ReadCheckpoint(&buf, &state, nil); err != nil {
		t.Fatal(err)
	}
	restored, err := Bind(rt, &state)
	if err != nil {
		t.Fatal(err)
	}
	ro := restored.TensorOperators[0]
	if state.Values[0] != state.LossInputs[0] || ro.Resolve(state.Values[0]) != ro.Resolve(state.LossInputs[0]) {
		t.Fatal("shared tensor identity lost")
	}
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	}()
	for _, c := range s.Components() {
		if c.Name() == "Driver" {
			c.(driver.Ticker).TickLater()
		}
	}
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if err := restored.Step(); err != nil {
		t.Fatal(err)
	}
	if len(ro.PendingFrees) != 1 {
		t.Fatalf("shared loss tensor released %d times", len(ro.PendingFrees))
	}
	if math.Abs(restored.LastLoss[0]+math.Log(0.75)) > 1e-8 {
		t.Fatal(restored.LastLoss)
	}
	if err := ro.CompleteStep(); err != nil {
		t.Fatal(err)
	}
}
