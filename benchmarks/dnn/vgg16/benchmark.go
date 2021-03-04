// Package vgg16 implements vgg16.
package vgg16

import (
	"gitlab.com/akita/dnn/dataset/cifar10"
	"gitlab.com/akita/dnn/layers"
	"gitlab.com/akita/dnn/training"
	"gitlab.com/akita/dnn/training/optimization"
	"gitlab.com/akita/mgpusim/benchmarks/dnn/tensor"
	"gitlab.com/akita/mgpusim/driver"
	"math"
)

// Benchmark defines the VGG16 network training benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	to      *tensor.GPUOperator
	gpus    []int

	network training.Network
	trainer training.Trainer
}

// NewBenchmark creates a new benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.to = tensor.NewGPUOperator(b.driver, b.context)
	//b.to.EnableVerification()

	b.network = training.Network{
		Layers: []layers.Layer{
			layers.NewConv2D(b.to, []int{3,32,32}, []int{64,3,3,3}, []int{1,1}, []int{1,1}),
			layers.NewReluLayer(b.to),
			layers.NewConv2D(b.to, []int{64,32,32}, []int{64,64,3,3}, []int{1,1}, []int{1,1}),
			layers.NewReluLayer(b.to),
			layers.NewMaxPoolingLayer(b.to, []int{2,2}, []int{0,0}, []int{2,2}),

			layers.NewConv2D(b.to, []int{64,16,16}, []int{128,64,3,3}, []int{1,1}, []int{1,1}),
			layers.NewReluLayer(b.to),
			layers.NewConv2D(b.to, []int{128,16,16}, []int{128,128,3,3}, []int{1,1}, []int{1,1}),
			layers.NewReluLayer(b.to),
			layers.NewMaxPoolingLayer(b.to, []int{2,2}, []int{0,0}, []int{2,2}),

			layers.NewConv2D(b.to, []int{128,8,8}, []int{256,128,3,3}, []int{1,1}, []int{1,1}),
			layers.NewReluLayer(b.to),
			layers.NewConv2D(b.to, []int{256,8,8}, []int{256,256,3,3}, []int{1,1}, []int{1,1}),
			layers.NewReluLayer(b.to),
			layers.NewConv2D(b.to, []int{256,8,8}, []int{256,256,3,3}, []int{1,1}, []int{1,1}),
			layers.NewReluLayer(b.to),
			layers.NewMaxPoolingLayer(b.to, []int{2,2}, []int{0,0}, []int{2,2}),

			layers.NewConv2D(b.to, []int{256,4,4}, []int{512,256,3,3}, []int{1,1}, []int{1,1}),
			layers.NewReluLayer(b.to),
			layers.NewConv2D(b.to, []int{512,4,4}, []int{512,512,3,3}, []int{1,1}, []int{1,1}),
			layers.NewReluLayer(b.to),
			layers.NewConv2D(b.to, []int{512,4,4}, []int{512,512,3,3}, []int{1,1}, []int{1,1}),
			layers.NewReluLayer(b.to),
			layers.NewMaxPoolingLayer(b.to, []int{2,2}, []int{0,0}, []int{2,2}),

			//layers.NewConv2D(b.to, []int{512,2,2}, []int{512,512,2,2}, []int{1,1}, []int{1,1}),
			//layers.NewReluLayer(b.to),
			//layers.NewConv2D(b.to, []int{512,2,2}, []int{512,512,2,2}, []int{1,1}, []int{1,1}),
			//layers.NewReluLayer(b.to),
			//layers.NewConv2D(b.to, []int{512,2,2}, []int{512,512,2,2}, []int{1,1}, []int{1,1}),
			//layers.NewReluLayer(b.to),
			//layers.NewMaxPoolingLayer(b.to, []int{2,2}, []int{0,0}, []int{2,2}),

			layers.NewFullyConnectedLayer(b.to, 2*2*512, 1*1*512),
			layers.NewReluLayer(b.to),
			layers.NewFullyConnectedLayer(b.to, 1*1*512, 10),
		},
	}

	b.trainer = training.Trainer{
		TO:              b.to,
		DataSource:      cifar10.NewTrainingDataSource(b.to),
		Network:         b.network,
		LossFunc:        training.NewSoftmaxCrossEntropy(b.to),
		OptimizationAlg: optimization.NewAdam(b.to, 0.001),
		Tester: &training.Tester{
			DataSource: cifar10.NewTestDataSource(b.to),
			Network:    b.network,
			BatchSize:  math.MaxInt32,
		},
		Epoch:         1,
		BatchSize:     8,
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
