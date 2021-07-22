// Package vgg16 implements VGG16 network training.
package vgg16

import (
	"math"

	"gitlab.com/akita/dnn/dataset/imagenet"
	"gitlab.com/akita/dnn/layers"
	"gitlab.com/akita/dnn/training"
	"gitlab.com/akita/dnn/training/optimization"
	"gitlab.com/akita/mgpusim/v2/benchmarks/dnn/gputraining"
	"gitlab.com/akita/mgpusim/v2/benchmarks/dnn/tensor"
	"gitlab.com/akita/mgpusim/v2/benchmarks/mccl"
	"gitlab.com/akita/mgpusim/v2/driver"
)

// Benchmark defines the VGG16 network training benchmark.
type Benchmark struct {
	driver   *driver.Driver
	ctx      *driver.Context
	to       []*tensor.GPUOperator
	gpus     []int
	contexts []*driver.Context

	networks []training.Network
	trainer  gputraining.DataParallelismMultiGPUTrainer

	BatchSize          int
	Epoch              int
	MaxBatchPerEpoch   int
	EnableTesting      bool
	EnableVerification bool
}

// NewBenchmark creates a new benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.ctx = driver.Init()

	return b
}

// SelectGPU selects the GPU to use.
func (b *Benchmark) SelectGPU(gpuIDs []int) {
	b.gpus = gpuIDs
}

func (b *Benchmark) init() {
	for _, gpu := range b.gpus {
		b.defineNetwork(gpu)
	}

	b.createTrainer()
	b.randomizeParams()
}

func (b *Benchmark) defineNetwork(gpuID int) {
	context := b.driver.InitWithExistingPID(b.ctx)
	b.driver.SelectGPU(context, gpuID)
	to := tensor.NewGPUOperator(b.driver, context)

	if b.EnableVerification {
		to.EnableVerification()
	}

	network := training.Network{
		Layers: []layers.Layer{
			layers.NewConv2D(to, []int{3, 224, 224}, []int{64, 3, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(to, []int{64, 224, 224}, []int{64, 64, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewMaxPoolingLayer(to, []int{2, 2}, []int{0, 0}, []int{2, 2}),

			layers.NewConv2D(to, []int{64, 112, 112}, []int{128, 64, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(to, []int{128, 112, 112}, []int{128, 128, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(to, []int{128, 112, 112}, []int{128, 128, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewMaxPoolingLayer(to, []int{2, 2}, []int{0, 0}, []int{2, 2}),

			layers.NewConv2D(to, []int{128, 56, 56}, []int{256, 128, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(to, []int{256, 56, 56}, []int{256, 256, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(to, []int{256, 56, 56}, []int{256, 256, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewMaxPoolingLayer(to, []int{2, 2}, []int{0, 0}, []int{2, 2}),

			layers.NewConv2D(to, []int{256, 28, 28}, []int{512, 256, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(to, []int{512, 28, 28}, []int{512, 512, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(to, []int{512, 28, 28}, []int{512, 512, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewMaxPoolingLayer(to, []int{2, 2}, []int{0, 0}, []int{2, 2}),

			layers.NewConv2D(to, []int{512, 14, 14}, []int{512, 512, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(to, []int{512, 14, 14}, []int{512, 512, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(to, []int{512, 14, 14}, []int{512, 512, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewMaxPoolingLayer(to, []int{2, 2}, []int{0, 0}, []int{2, 2}),

			layers.NewFullyConnectedLayer(to, 7*7*512, 2*2*512),
			layers.NewReluLayer(to),
			layers.NewFullyConnectedLayer(to, 2*2*512, 200),
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

	b.trainer = gputraining.DataParallelismMultiGPUTrainer{
		TensorOperators:  b.to,
		DataSource:       sources,
		Networks:         b.networks,
		LossFunc:         lossFuncs,
		OptimizationAlg:  alg,
		Tester:           testers,
		Epoch:            b.Epoch,
		MaxBatchPerEpoch: b.MaxBatchPerEpoch,
		BatchSize:        b.BatchSize,
		ShowBatchInfo:    true,
		GPUs:             b.gpus,
		Contexts:         b.contexts,
		Driver:           b.driver,
	}
}

func (b *Benchmark) randomizeParams() {
	initNet := b.networks[0]
	for _, l := range initNet.Layers {
		l.Randomize()
	}

	gpuNum := len(b.networks)

	for i := range b.networks[0].Layers {
		if b.networks[0].Layers[i].Parameters() == nil {
			continue
		}

		params := make([]*tensor.Tensor, gpuNum)
		datas := make([]driver.Ptr, gpuNum)

		for j := 0; j < gpuNum; j++ {
			params[j] = b.networks[j].Layers[i].Parameters().(*tensor.Tensor)
		}

		dataSizeArr := params[0].Size()
		dataSize := 1
		for i := 0; i < len(dataSizeArr); i++ {
			dataSize *= dataSizeArr[i]
		}

		for i := 0; i < len(params); i++ {
			datas[i] = params[i].Ptr()
		}
		comms := mccl.CommInitAllMultipleContexts(
			gpuNum, b.driver, b.contexts, b.gpus)
		mccl.BroadcastRing(b.driver, comms, 1, datas, dataSize)
	}
}

// Run executes the benchmark.
func (b *Benchmark) Run() {
	b.init()
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
