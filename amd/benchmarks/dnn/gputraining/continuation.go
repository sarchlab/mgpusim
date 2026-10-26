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
	Images   *imagenet.State
	Resident *training.DataState
}

func sourceState(d training.DataSource) (SourceState, error) {
	if d, ok := d.(*imagenet.DataSource); ok {
		return SourceState{Images: d.State}, nil
	}
	if d, ok := d.(training.SavedDataSource); ok {
		return SourceState{Resident: d.SavedData()}, nil
	}
	return SourceState{}, fmt.Errorf("unsupported saved data source %T", d)
}
func bindSource(o *gputensor.GPUOperator, s SourceState) (training.DataSource, error) {
	if s.Images != nil {
		return imagenet.Bind(o, s.Images)
	}
	if s.Resident == nil {
		return nil, fmt.Errorf("missing resident data")
	}
	return training.BindResident(o, s.Resident)
}

type GPUState struct {
	Operator      *gputensor.OperatorState
	Layers        []layers.State
	Source        SourceState
	Loss          string
	Optimizer     *optimization.State
	HasTester     bool
	TestSource    SourceState
	TestBatchSize int
}

// State is the trainer's actual continuation, including the canonical graph.
type State struct {
	GPUs                                []int
	Contexts                            []driver.Context
	Epoch, MaxBatchPerEpoch, BatchSize  int
	ShowBatchInfo                       bool
	Networks                            []GPUState
	Phase                               string
	CurrentEpoch, Batch, Layer, TestGPU int
	Ended                               bool
	Values, LossInputs, Derivatives     []tensor.ID
	Labels                              [][]int
	Readbacks, LossReadbacks            []driver.HostBuffer
	Collective                          *mccl.Collective
	Buffers                             []driver.Ptr
	LastLoss, LastAccuracy              []float64
}

// Connect registers a newly constructed graph's actual states exactly once.
// It is not used by SaveCheckpoint or by subsequent steps.
func (t *DataParallelismMultiGPUTrainer) Connect() error {
	t.State.Networks = nil
	for i := range t.Networks {
		g, err := t.connectGPU(i)
		if err != nil {
			return err
		}
		t.State.Networks = append(t.State.Networks, g)
	}
	return nil
}

// Bind reconstructs only runtime/interface bindings to a saved graph.
func Bind(r *driver.Runtime, s *State) (DataParallelismMultiGPUTrainer, error) {
	t := DataParallelismMultiGPUTrainer{State: s, Driver: r}
	if s == nil {
		return t, fmt.Errorf("missing trainer state")
	}
	for _, g := range s.Networks {
		if err := t.bindGPU(g); err != nil {
			return t, err
		}
	}
	return t, nil
}
func (t *DataParallelismMultiGPUTrainer) connectGPU(i int) (GPUState, error) {
	n := t.Networks[i]
	g := GPUState{Operator: t.TensorOperators[i].OperatorState}
	var err error
	for _, l := range n.Layers {
		state, e := layers.StateOf(l)
		if e != nil {
			return GPUState{}, e
		}
		g.Layers = append(g.Layers, state)
	}
	g.Source, err = sourceState(t.DataSource[i])
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
	g.Optimizer, err = optimization.StateOf(t.OptimizationAlg[i])
	if err != nil {
		return GPUState{}, err
	}
	if i < len(t.Tester) && t.Tester[i] != nil {
		g.HasTester = true
		g.TestBatchSize = t.Tester[i].BatchSize
		g.TestSource, err = sourceState(t.Tester[i].DataSource)
		if err != nil {
			return GPUState{}, err
		}
	}
	return g, nil
}

func (t *DataParallelismMultiGPUTrainer) bindGPU(g GPUState) error {
	o := gputensor.BindOperator(t.Driver, g.Operator)
	n := training.Network{}
	for _, saved := range g.Layers {
		l, err := layers.Bind(o, saved)
		if err != nil {
			return err
		}
		n.Layers = append(n.Layers, l)
	}
	d, err := bindSource(o, g.Source)
	if err != nil {
		return err
	}
	alg, err := optimization.Bind(o, g.Optimizer)
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
		source, err := bindSource(o, g.TestSource)
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
