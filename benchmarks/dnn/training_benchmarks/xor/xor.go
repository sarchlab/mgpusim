// Package xor implements a extremely simple network that can perform the xor
// operation.
package xor

import (
	"fmt"

	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/training"
	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/training/optimization"
	"github.com/sarchlab/mgpusim/v3/driver"
)

// Benchmark defines the XOR network training benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	to      *gputensor.GPUOperator

	gpus []int

	network training.Network
	trainer training.Trainer
}

// NewBenchmark creates a new benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.to = gputensor.NewGPUOperator(b.driver, b.context)
	b.to.EnableVerification()

	b.network = training.Network{
		Layers: []layers.Layer{
			layers.NewFullyConnectedLayer(
				0,
				b.to,
				2, 4,
			),
			layers.NewReluLayer(b.to),
			layers.NewFullyConnectedLayer(
				2,
				b.to,
				4, 2,
			),
		},
	}

	b.trainer = training.Trainer{
		TO:              b.to,
		DataSource:      NewDataSource(b.to),
		Network:         b.network,
		LossFunc:        training.NewSoftmaxCrossEntropy(b.to),
		OptimizationAlg: optimization.NewAdam(b.to, 0.03),
		Epoch:           50,
		BatchSize:       4,
		ShowBatchInfo:   true,
	}

	b.enableLayerVerification(&b.network)

	return b
}

func (b *Benchmark) enableLayerVerification(network *training.Network) {

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

func (b *Benchmark) printLayerParams() {
	for i, l := range b.network.Layers {
		params := l.Parameters()
		if params != nil {
			fmt.Println("Layer ", i, params.Vector())
		}
	}
}

// Verify runs the benchmark on the CPU and checks the result.
func (b *Benchmark) Verify() {
	panic("not implemented")
}

// SetUnifiedMemory asks the benchmark to use unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	panic("unified memory is not supported by dnn workloads")
}
