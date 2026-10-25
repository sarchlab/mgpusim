package gputraining

import (
	"fmt"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/dataset/imagenet"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training/optimization"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/mccl"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type SourceState struct {
	ImageNet bool
	Images   imagenet.State
	Resident training.DataState
}

func snapshotSource(d training.DataSource) (SourceState, error) {
	if d, ok := d.(*imagenet.DataSource); ok {
		return SourceState{ImageNet: true, Images: d.Snapshot()}, nil
	}
	if d, ok := d.(interface{ SnapshotData() training.DataState }); ok {
		return SourceState{Resident: d.SnapshotData()}, nil
	}
	return SourceState{}, fmt.Errorf("data source %T has no saved continuation", d)
}
func restoreSource(o *gputensor.GPUOperator, s SourceState) (training.DataSource, error) {
	if s.ImageNet {
		return imagenet.Restore(o, s.Images)
	}
	return &training.ResidentDataSource{State: s.Resident, Operator: o}, nil
}

type GPUState struct {
	Operator      gputensor.OperatorState
	Layers        []layers.State
	Source        SourceState
	Loss          string
	Optimizer     optimization.State
	HasTester     bool
	TestSource    SourceState
	TestBatchSize int
}

// State includes every host-side training continuation. Simulated memory and
// pending commands are separately owned by the simulation's resources.
type State struct {
	GPUs                                []int
	Contexts                            []driver.Context
	Epoch, MaxBatchPerEpoch, BatchSize  int
	ShowBatchInfo                       bool
	Networks                            []GPUState
	Phase                               string
	CurrentEpoch, Batch, Layer, TestGPU int
	Ended                               bool
	Values, LossInputs, Derivatives     []gputensor.TensorState
	Labels                              [][]int
	Readbacks, LossReadbacks            []driver.CommandID
	Collective                          *mccl.Collective
	Buffers                             []driver.Ptr
	LastLoss, LastAccuracy              []float64
}

func snapshotTensors(ts []tensor.Tensor) []gputensor.TensorState {
	out := make([]gputensor.TensorState, len(ts))
	for i, t := range ts {
		out[i] = gputensor.Snapshot(t)
	}
	return out
}
func (t *DataParallelismMultiGPUTrainer) Snapshot() (State, error) {
	s := State{
		GPUs:             t.GPUs,
		Contexts:         t.Contexts,
		Epoch:            t.Epoch,
		MaxBatchPerEpoch: t.MaxBatchPerEpoch,
		BatchSize:        t.BatchSize,
		ShowBatchInfo:    t.ShowBatchInfo,
		Phase:            t.phase,
		CurrentEpoch:     t.epoch,
		Batch:            t.batch,
		Layer:            t.layer,
		TestGPU:          t.testGPU,
		Ended:            t.ended,
		Values:           snapshotTensors(t.values),
		LossInputs:       snapshotTensors(t.lossInputs),
		Derivatives:      snapshotTensors(t.derivatives),
		Labels:           t.labels,
		Readbacks:        t.readbacks,
		LossReadbacks:    t.lossReadbacks,
		Collective:       t.collective,
		Buffers:          t.buffers,
		LastLoss:         t.LastLoss,
		LastAccuracy:     t.LastAccuracy,
	}
	for i := range t.Networks {
		g, err := t.snapshotGPU(i)
		if err != nil {
			return State{}, err
		}
		s.Networks = append(s.Networks, g)
	}

	return s, nil
}
func Restore(r *driver.Runtime, s State) (DataParallelismMultiGPUTrainer, error) {
	t := DataParallelismMultiGPUTrainer{
		Driver:           r,
		GPUs:             s.GPUs,
		Contexts:         s.Contexts,
		Epoch:            s.Epoch,
		MaxBatchPerEpoch: s.MaxBatchPerEpoch,
		BatchSize:        s.BatchSize,
		ShowBatchInfo:    s.ShowBatchInfo,
		phase:            s.Phase,
		epoch:            s.CurrentEpoch,
		batch:            s.Batch,
		layer:            s.Layer,
		testGPU:          s.TestGPU,
		ended:            s.Ended,
		labels:           s.Labels,
		readbacks:        s.Readbacks,
		lossReadbacks:    s.LossReadbacks,
		collective:       s.Collective,
		buffers:          s.Buffers,
		LastLoss:         s.LastLoss,
		LastAccuracy:     s.LastAccuracy,
	}
	for _, g := range s.Networks {
		if err := t.restoreGPU(g); err != nil {
			return t, err
		}
	}

	t.restoreContinuation(s)
	return t, nil
}

func (t *DataParallelismMultiGPUTrainer) restoreContinuation(s State) {
	restore := func(states []gputensor.TensorState) []tensor.Tensor {
		out := make([]tensor.Tensor, len(states))
		for i, v := range states {
			index := i
			if s.Phase == "testForward" || s.Phase == "testRead" {
				index = s.TestGPU
			}
			out[i] = t.TensorOperators[index].RestoreTensor(v)
		}
		return out
	}
	t.values = restore(s.Values)
	t.lossInputs = restore(s.LossInputs)
	t.derivatives = restore(s.Derivatives)
	// CrossEntropy uses the output itself as the loss input. Preserve that host
	// ownership relation so the post-read phase releases the allocation once.
	if s.Phase == "lossRead" {
		for i := range t.values {
			if s.Networks[i].Loss == "crossentropy" {
				t.lossInputs[i] = t.values[i]
			}
		}
	}
}

func (t *DataParallelismMultiGPUTrainer) snapshotGPU(i int) (GPUState, error) {
	n := t.Networks[i]
	g := GPUState{Operator: t.TensorOperators[i].Snapshot()}
	var err error
	for _, l := range n.Layers {
		state, e := layers.Snapshot(l)
		if e != nil {
			return GPUState{}, e
		}
		g.Layers = append(g.Layers, state)
	}
	g.Source, err = snapshotSource(t.DataSource[i])
	if err != nil {
		return GPUState{}, err
	}
	switch t.LossFunc[i].(type) {
	case *training.SoftmaxCrossEntropy:
		g.Loss = "softmax"
	case *training.CrossEntropy:
		g.Loss = "crossentropy"
	default:
		return GPUState{}, fmt.Errorf("unsupported loss %T", t.LossFunc[i])
	}
	g.Optimizer, err = optimization.Snapshot(t.OptimizationAlg[i], n.Layers)
	if err != nil {
		return GPUState{}, err
	}
	if i < len(t.Tester) && t.Tester[i] != nil {
		g.HasTester = true
		g.TestBatchSize = t.Tester[i].BatchSize
		g.TestSource, err = snapshotSource(t.Tester[i].DataSource)
		if err != nil {
			return GPUState{}, err
		}
	}
	return g, nil
}

func (t *DataParallelismMultiGPUTrainer) restoreGPU(g GPUState) error {
	o := gputensor.RestoreOperator(t.Driver, g.Operator)
	n := training.Network{}
	for _, saved := range g.Layers {
		l, err := layers.Restore(o, saved)
		if err != nil {
			return err
		}
		n.Layers = append(n.Layers, l)
	}
	d, err := restoreSource(o, g.Source)
	if err != nil {
		return err
	}
	alg, err := optimization.Restore(o, n.Layers, g.Optimizer)
	if err != nil {
		return err
	}
	var loss training.LossFunction
	switch g.Loss {
	case "softmax":
		loss = training.NewSoftmaxCrossEntropy(o)
	case "crossentropy":
		loss = training.NewCrossEntropy(o)
	default:
		return fmt.Errorf("unknown saved loss %q", g.Loss)
	}
	var tester *training.Tester
	if g.HasTester {
		source, err := restoreSource(o, g.TestSource)
		if err != nil {
			return err
		}
		tester = &training.Tester{DataSource: source, Network: n, BatchSize: g.TestBatchSize}
	}
	t.TensorOperators = append(t.TensorOperators, o)
	t.Networks = append(t.Networks, n)
	t.DataSource = append(t.DataSource, d)
	t.OptimizationAlg = append(t.OptimizationAlg, alg)
	t.LossFunc = append(t.LossFunc, loss)
	t.Tester = append(t.Tester, tester)
	return nil
}
