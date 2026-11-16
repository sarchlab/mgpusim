// Package vgg16 implements VGG16 network training.
package vgg16

import (
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/dataset/imagenet"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputraining"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training/optimization"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// Benchmark defines the VGG16 network training benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt      *gpuruntime.Runtime
	trainer gputraining.DataParallelismMultiGPUTrainer

	to       []*gputensor.GPUOperator
	networks []training.Network
	contexts []gpuruntime.Context
}

// NewBenchmark creates a new benchmark.
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = rt.Init()

	return b
}

// SelectGPU selects the GPU to use.
func (b *Benchmark) SelectGPU(gpuIDs []int) {
	b.GPUs = gpuIDs
}

func (b *Benchmark) init() error {
	for _, gpu := range b.GPUs {
		b.defineNetwork(gpu)
	}

	b.createTrainer()
	return b.trainer.Connect()
}

func (b *Benchmark) defineNetwork(gpuID int) {
	context := b.rt.InitWithExistingPID(b.Context)
	b.rt.SelectGPU(context, gpuID)
	to := gputensor.NewGPUOperator(b.rt, context)
	to.Arch = b.Arch

	if b.EnableVerification {
		to.EnableVerification()
	}

	network := training.Network{
		Layers: []layers.Layer{
			layers.NewConv2D(0, to, []int{3, 224, 224}, []int{64, 3, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(2, to, []int{64, 224, 224}, []int{64, 64, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewMaxPoolingLayer(to, []int{2, 2}, []int{0, 0}, []int{2, 2}),

			layers.NewConv2D(5, to, []int{64, 112, 112}, []int{128, 64, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(7, to, []int{128, 112, 112}, []int{128, 128, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(9, to, []int{128, 112, 112}, []int{128, 128, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewMaxPoolingLayer(to, []int{2, 2}, []int{0, 0}, []int{2, 2}),

			layers.NewConv2D(12, to, []int{128, 56, 56}, []int{256, 128, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(14, to, []int{256, 56, 56}, []int{256, 256, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(16, to, []int{256, 56, 56}, []int{256, 256, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewMaxPoolingLayer(to, []int{2, 2}, []int{0, 0}, []int{2, 2}),

			layers.NewConv2D(19, to, []int{256, 28, 28}, []int{512, 256, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(21, to, []int{512, 28, 28}, []int{512, 512, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(23, to, []int{512, 28, 28}, []int{512, 512, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewMaxPoolingLayer(to, []int{2, 2}, []int{0, 0}, []int{2, 2}),

			layers.NewConv2D(26, to, []int{512, 14, 14}, []int{512, 512, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(27, to, []int{512, 14, 14}, []int{512, 512, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(29, to, []int{512, 14, 14}, []int{512, 512, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewMaxPoolingLayer(to, []int{2, 2}, []int{0, 0}, []int{2, 2}),

			layers.NewFullyConnectedLayer(32, to, 7*7*512, 2*2*512),
			layers.NewReluLayer(to),
			layers.NewFullyConnectedLayer(34, to, 2*2*512, 200),
		},
	}

	b.networks = append(b.networks, network)
	b.contexts = append(b.contexts, context)
	b.to = append(b.to, to)
}

func (b *Benchmark) createTrainer() {
	sources := make([]training.DataSource, len(b.networks))
	alg := make([]optimization.Alg, len(b.networks))
	testers := make([]*training.Tester, len(b.networks))
	lossFuncs := make([]training.LossFunction, len(b.networks))

	for i := 0; i < len(b.networks); i++ {
		sources[i] = imagenet.NewTrainingDataSource(b.to[i])
		alg[i] = optimization.NewAdam(b.to[i], 0.001)
		lossFuncs[i] = training.NewSoftmaxCrossEntropy(b.to[i])

		if b.EnableTesting {
			testers[i] = &training.Tester{
				DataSource: imagenet.NewTestDataSource(b.to[i]),
				Network:    b.networks[i],
				BatchSize:  math.MaxInt32,
			}
		}
	}

	b.Trainer = gputraining.State{
		Epoch:            b.Epoch,
		MaxBatchPerEpoch: b.MaxBatchPerEpoch,
		BatchSize:        b.BatchSize,
		ShowBatchInfo:    true,
		GPUs:             b.GPUs,
		Contexts:         b.contexts,
	}
	b.trainer = gputraining.DataParallelismMultiGPUTrainer{State: &b.Trainer,
		TensorOperators: b.to,
		DataSource:      sources,
		Networks:        b.networks,
		LossFunc:        lossFuncs,
		OptimizationAlg: alg,
		Tester:          testers,
		Runtime:         b.rt,
	}
}

// Step submits one training phase after the previous global batch drained.
func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.rt); err != nil {
		return err
	}
	if b.Ended {
		return nil
	}
	if b.Phase == 0 {
		if err := b.init(); err != nil {
			return err
		}
		b.Phase = 1
	}
	if err := b.trainer.Step(); err != nil {
		return err
	}
	b.Ended = b.trainer.IsEnded()
	return nil
}
func (b *Benchmark) SetVerification(enabled bool) {
	b.Progress.SetVerification(enabled)
	b.EnableVerification = enabled
}

// SetUnifiedMemory asks the benchmark to use unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	panic("unified memory is not supported by dnn workloads")
}
