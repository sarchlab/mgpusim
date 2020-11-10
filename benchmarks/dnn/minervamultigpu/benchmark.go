// Package minervamultigpu implements minerva network training on multi-GPU
// systems.
package minervamultigpu

import (
	"math"

	"gitlab.com/akita/dnn/dataset/mnist"
	"gitlab.com/akita/dnn/layers"
	"gitlab.com/akita/dnn/training"
	"gitlab.com/akita/dnn/training/optimization"
	"gitlab.com/akita/mgpusim/benchmarks/dnn/gputraining"
	simLayers "gitlab.com/akita/mgpusim/benchmarks/dnn/layers"
	"gitlab.com/akita/mgpusim/driver"
)

// Benchmark defines the Mineva network training benchmark.
type Benchmark struct {
	driver *driver.Driver
	gpus   []int

	networks []training.Network
	trainer  gputraining.DataParallelismMultiGPUTrainer
}

// NewBenchmark creates a new benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver

	// b.enableLayerVerification(&b.network)

	return b
}

func (b *Benchmark) enableLayerVerification(network *training.Network) {
	network.Layers[1].(*simLayers.FullyConnectedLayer).EnableVerification()
	network.Layers[2].(*simLayers.ReluLayer).EnableVerification()
	network.Layers[3].(*simLayers.FullyConnectedLayer).EnableVerification()
	network.Layers[4].(*simLayers.ReluLayer).EnableVerification()
	network.Layers[5].(*simLayers.FullyConnectedLayer).EnableVerification()
	network.Layers[6].(*simLayers.ReluLayer).EnableVerification()
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
	context := b.driver.Init()
	b.driver.SelectGPU(context, gpuID)
	to := simLayers.NewTensorOperator(b.driver, context)

	network := training.Network{
		Layers: []layers.Layer{
			simLayers.CPUToGPULayer{
				GPUDriver: b.driver,
				GPUCtx:    context,
			},
			simLayers.NewFullyConnectedLayer(
				784, 256,
				b.driver, context,
				to,
			),
			simLayers.NewReluLayer(b.driver, context),
			simLayers.NewFullyConnectedLayer(
				256, 100,
				b.driver, context,
				to,
			),
			simLayers.NewReluLayer(b.driver, context),
			simLayers.NewFullyConnectedLayer(
				100, 100,
				b.driver, context,
				simLayers.NewTensorOperator(b.driver, context),
			),
			simLayers.NewReluLayer(b.driver, context),
			simLayers.NewFullyConnectedLayer(
				100, 10,
				b.driver, context,
				simLayers.NewTensorOperator(b.driver, context),
			),
			simLayers.GPUToCPULayer{
				GPUDriver: b.driver,
				GPUCtx:    context,
			},
		},
	}

	b.networks = append(b.networks, network)
}

func (b *Benchmark) createTrainer() {
	b.trainer = gputraining.DataParallelismMultiGPUTrainer{
		DataSource:      mnist.NewTrainingDataSource(),
		Networks:        b.networks,
		LossFunc:        training.SoftmaxCrossEntropy{},
		OptimizationAlg: optimization.NewAdam(0.001),
		Tester: &training.Tester{
			DataSource: mnist.NewTestDataSource(),
			Network:    b.networks[0],
			BatchSize:  math.MaxInt32,
		},
		Epoch:         1,
		BatchSize:     128,
		ShowBatchInfo: true,
	}
}

func (b *Benchmark) randomizeParams() {
	//TODO: this is a very bad implementation
	for _, n := range b.networks {
		for _, l := range n.Layers {
			l.Randomize()
		}
	}

	for i := 1; i < len(b.networks); i++ {
		for l := range b.networks[0].Layers {
			srcParam := b.networks[0].Layers[l].Parameters()
			if srcParam != nil {
				dstParam := b.networks[i].Layers[l].Parameters()
				dstParam.Scale(0)
				dstParam.Add(srcParam)
			}
		}
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
