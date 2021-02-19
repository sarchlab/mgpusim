// Package lenet defines the lenet training benchmark.
package lenet

import (
	"math"

	"gitlab.com/akita/dnn/dataset/mnist"
	"gitlab.com/akita/dnn/layers"
	"gitlab.com/akita/dnn/training"
	"gitlab.com/akita/dnn/training/optimization"
	"gitlab.com/akita/mgpusim/benchmarks/dnn/tensor"
	"gitlab.com/akita/mgpusim/driver"
)

// Benchmark defines a benchmark that trains LeNet over the MNIST dataset.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	to      *tensor.GPUOperator
	gpus    []int

	network training.Network
	trainer training.Trainer
}

// NewBenchmark creates a new LeNet training benchmark.
//https://www.datasciencecentral.com/profiles/blogs/lenet-5-a-classic-cnn-architecture
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.to = tensor.NewGPUOperator(b.driver, b.context)
	b.to.EnableVerification()

	b.network = training.Network{
		Layers: []layers.Layer{
			layers.NewConv2D(
				b.to,
				[]int{1, 28, 28},
				[]int{6, 1, 5, 5},
				[]int{1, 1},
				[]int{2, 2},
			),
			layers.NewReluLayer(b.to),
			layers.NewMaxPoolingLayer(
				b.to,
				[]int{2, 2},
				[]int{0, 0},
				[]int{2, 2},
			),
			layers.NewConv2D(b.to,
				[]int{6, 14, 14},
				[]int{16, 6, 5, 5},
				[]int{1, 1},
				[]int{0, 0}),
			layers.NewReluLayer(b.to),
			layers.NewMaxPoolingLayer(b.to,
				[]int{2, 2},
				[]int{0, 0},
				[]int{2, 2}),
			layers.NewFullyConnectedLayer(b.to, 400, 120),
			layers.NewReluLayer(b.to),
			layers.NewFullyConnectedLayer(b.to, 120, 84),
			layers.NewReluLayer(b.to),
			layers.NewFullyConnectedLayer(b.to, 84, 10),
		},
	}

	b.trainer = training.Trainer{
		TO:              b.to,
		DataSource:      mnist.NewTrainingDataSource(b.to),
		Network:         b.network,
		LossFunc:        training.NewSoftmaxCrossEntropy(b.to),
		OptimizationAlg: optimization.NewAdam(b.to, 0.001),
		Tester: &training.Tester{
			DataSource: mnist.NewTestDataSource(b.to),
			Network:    b.network,
			BatchSize:  math.MaxInt32,
		},
		Epoch:         1,
		BatchSize:     16,
		ShowBatchInfo: true,
	}

	return b
}

// SelectGPU selects the GPU to use.
func (b *Benchmark) SelectGPU(gpuIDs []int) {
	if len(gpuIDs) > 1 {
		panic("multi-GPU is not supported by DNN workloads")
	}
}

// Run executes the benchmark.
func (b *Benchmark) Run() {
	for _, l := range b.network.Layers {
		l.Randomize()
	}
	b.trainer.Train()
}

// Verify runs the benchmark on the CPU and checks the result.
func (b *Benchmark) Verify() {
	panic("not implemented")
}

// SetUnifiedMemory asks the benchmark to use unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	panic("unified memory is not supported by dnn workloads")
}
