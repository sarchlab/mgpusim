// Package maxpooling implements the maxpooling algorithm as a benchmark.
package maxpooling

import (
	"math"
	"math/rand"

	"gitlab.com/akita/mgpusim/benchmarks/dnn/layers"
	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
)

// Parameters defines the parameters of the maxpooling benchmark.
type Parameters struct {
	N, C, H, W       int
	KernelH, KernelW int
	StrideH, StrideW int
	PadH, PadW       int
}

// PooledH returns the height of the output image.
func (p Parameters) PooledH() int {
	return int(math.Ceil(float64(p.H+2*p.PadH-p.KernelH)/float64(p.StrideH))) + 1
}

// PooledW returns the width of the output image.
func (p Parameters) PooledW() int {
	return int(math.Ceil(float64(p.W+2*p.PadW-p.KernelW)/float64(p.StrideW))) + 1
}

// InputLength returns the length of the input data.
func (p Parameters) InputLength() int {
	return p.N * p.C * p.H * p.W
}

// OutputLength returns the length of the output data.
func (p Parameters) OutputLength() int {
	return p.N * p.C * p.PooledH() * p.PooledW()
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	hsaco   *insts.HsaCo

	parameters Parameters
	layer      *layers.MaxPoolingLayer

	useUnifiedMemory bool
}

// NewBenchmark makes a new benchmark
func NewBenchmark(
	driver *driver.Driver,
	parameters Parameters,
) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = driver.Init()

	b.parameters = parameters
	b.layer = layers.NewMaxPoolingLayer(
		[2]int{b.parameters.StrideH, b.parameters.StrideW},
		[2]int{b.parameters.PadH, b.parameters.PadW},
		[2]int{b.parameters.KernelH, b.parameters.KernelW},
		b.driver, b.context,
	)

	hsacoBytes := _escFSMustByte(false, "/kernels.hsaco")

	b.hsaco = kernels.LoadProgramFromMemory(hsacoBytes, "MaxPoolForward")

	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// EnableVerification will ask the layer to check the results after running the
// forward and backward pass.
func (b *Benchmark) EnableVerification() {
	b.layer.EnableVerification()
}

// Run runs
func (b *Benchmark) Run() {
	b.driver.SelectGPU(b.context, b.gpus[0])
	b.exec()
}

func (b *Benchmark) exec() {
	forwardIn := make([]float64, b.parameters.InputLength())
	for i := 0; i < b.parameters.InputLength(); i++ {
		// forwardIn[i] = rand.NormFloat64()
		forwardIn[i] = float64(-i)
	}

	backwardIn := make([]float64, b.parameters.OutputLength())
	for i := 0; i < b.parameters.OutputLength(); i++ {
		backwardIn[i] = rand.NormFloat64()
	}

	forwardInputTensor := layers.NewTensor(b.driver, b.context)
	forwardInputTensor.Init(forwardIn, []int{
		b.parameters.N,
		b.parameters.C,
		b.parameters.H,
		b.parameters.W,
	})

	backwardInputTensor := layers.NewTensor(b.driver, b.context)
	backwardInputTensor.Init(backwardIn, []int{
		b.parameters.N,
		b.parameters.C,
		b.parameters.PooledH(),
		b.parameters.PooledW(),
	})

	b.layer.Forward(forwardInputTensor)
	b.layer.Backward(backwardInputTensor)
}

// Verify verifies
func (b *Benchmark) Verify() {
	// Do nothing
}
