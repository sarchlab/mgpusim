// Package im2col defines a benchmark for the im2col operation.
package im2col

import (
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// A Benchmark is a benchmark for the im2col operation.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt       *gpuruntime.Runtime
	operator *gputensor.GPUOperator
}

// NewBenchmark creates a new Im2Col benchmark. It requires the GPU Runtime as an
// argument.
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := &Benchmark{
		rt: rt, State: &State{},
	}

	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)
	b.Context = b.rt.Init()

	return b
}

// EnableVerification configures the benchmark to verify the result.
func (b *Benchmark) EnableVerification() {
	b.SetVerification(true)
}

// SelectGPU selects the GPU to run the benchmark on.
func (b *Benchmark) SelectGPU(gpus []int) {
	if len(gpus) > 1 {
		panic("Im2Col benchmark can only run on a single GPU for now.")
	}

	b.Gpus = gpus
}

// SetUnifiedMemory configures the benchmark to use unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) submit() {
	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.operator = gputensor.NewGPUOperator(b.rt, b.Context)
	b.Operator = b.operator.OperatorState
	b.operator.Arch = b.Arch
	b.operator.ReportTime()
	b.operator.SetVerification(b.Verification)
	b.calculateOutputSize()
	b.initMem()
	b.exec()
}

func (b *Benchmark) calculateOutputSize() {
	b.OutputH = (b.H+2*b.PadY-b.KernelHeight)/b.StrideY + 1
	b.OutputW = (b.W+2*b.PadX-b.KernelWidth)/b.StrideX + 1
}

func (b *Benchmark) initMem() {
	input := make([]float64, b.N*b.C*b.H*b.W)

	for i := 0; i < b.N*b.C*b.H*b.W; i++ {
		input[i] = b.operator.RandomFloat64()
	}

	b.InputID = b.operator.Reference(b.operator.CreateWithData(
		input, []int{b.N, b.C, b.H, b.W}, "NCHW"))
}

func (b *Benchmark) exec() {
	b.operator.Im2Col(b.operator.Resolve(b.InputID),
		[]int{b.KernelHeight, b.KernelWidth},
		[]int{b.PadY, b.PadX},
		[]int{b.StrideY, b.StrideX},
		[]int{b.DilateY, b.DilateX},
	)
}

func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.rt); err != nil {
		return err
	}
	switch b.Phase {
	case 0:
		b.submit()
		b.Phase = 1
	case 1:
		if err := b.operator.CompleteStep(); err != nil {
			return err
		}
		b.Ended = true
	}
	return nil
}
