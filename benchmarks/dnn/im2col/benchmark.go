// Package im2col defines a benchmark for the im2col operation.
package im2col

import (
	"math/rand"

	"gitlab.com/akita/dnn/tensor"
	gpuTensor "gitlab.com/akita/mgpusim/v3/benchmarks/dnn/tensor"
	"gitlab.com/akita/mgpusim/v3/driver"
)

// A Benchmark is a benchmark for the im2col operation.
type Benchmark struct {
	driver           *driver.Driver
	context          *driver.Context
	gpus             []int
	useUnifiedMemory bool

	N, C, H, W                int
	outputH, outputW          int
	KernelHeight, KernelWidth int
	PadX, PadY                int
	StrideX, StrideY          int
	DilateX, DilateY          int

	operator *gpuTensor.GPUOperator

	Input tensor.Tensor
}

// NewBenchmark creates a new Im2Col benchmark. It requires the GPU driver as an
// argument.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := &Benchmark{
		driver: driver,
	}

	b.context = b.driver.Init()
	b.operator = gpuTensor.NewGPUOperator(b.driver, b.context)
	b.operator.ReportTime()

	return b
}

// EnableVerification configures the benchmark to verify the result.
func (b *Benchmark) EnableVerification() {
	b.operator.EnableVerification()
}

// SelectGPU selects the GPU to run the benchmark on.
func (b *Benchmark) SelectGPU(gpus []int) {
	if len(gpus) > 1 {
		panic("Im2Col benchmark can only run on a single GPU for now.")
	}

	b.gpus = gpus
}

// SetUnifiedMemory configures the benchmark to use unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// Run runs the benchmark.
func (b *Benchmark) Run() {
	b.driver.SelectGPU(b.context, b.gpus[0])
	b.calculateOutputSize()
	b.initMem()
	b.exec()
}

func (b *Benchmark) calculateOutputSize() {
	b.outputH = (b.H+2*b.PadY-b.KernelHeight)/b.StrideY + 1
	b.outputW = (b.W+2*b.PadX-b.KernelWidth)/b.StrideX + 1
}

func (b *Benchmark) initMem() {
	input := make([]float64, b.N*b.C*b.H*b.W)

	for i := 0; i < b.N*b.C*b.H*b.W; i++ {
		input[i] = rand.Float64()
	}

	b.Input = b.operator.CreateWithData(
		input, []int{b.N, b.C, b.H, b.W}, "NCHW")
}

func (b *Benchmark) exec() {
	b.operator.Im2Col(b.Input,
		[]int{b.KernelHeight, b.KernelWidth},
		[]int{b.PadY, b.PadX},
		[]int{b.StrideY, b.StrideX},
		[]int{b.DilateY, b.DilateX},
	)
}

// Verify does nothing for now.
func (b *Benchmark) Verify() {
}
