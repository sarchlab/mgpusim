// Package conv2d defines a benchmark for the Convolutional Layer.
package conv2d

import (
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/layers"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// A Benchmark is a benchmark for the Convolutional Layer.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt       *gpuruntime.Runtime
	operator *gputensor.GPUOperator
	layer    *layers.Conv2D
}

// NewBenchmark creates a new Conv2D benchmark. It requires the GPU Runtime as an argument.
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
		panic("Conv2D benchmark can only run on a single GPU for now.")
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
	b.layer = layers.NewConv2D(
		0,
		b.operator,
		[]int{b.C, b.H, b.W},
		[]int{b.KernelChannel, b.C, b.KernelHeight, b.KernelWidth},
		[]int{b.StrideY, b.StrideX},
		[]int{b.PadY, b.PadX},
	)
	b.Layer = layers.State{Kind: "Conv2D", Conv2D: b.layer.Conv2DState}
	b.layer.Randomize()

	b.ForwardInID = b.operator.Reference(b.operator.Zeros([]int{b.N, b.C, b.H, b.W}))

	if b.EnableBackward {
		b.BackwardInID = b.operator.Reference(b.operator.Zeros(
			[]int{b.N, b.KernelChannel, b.OutputH, b.OutputW}))
	}
}

func (b *Benchmark) exec() {
	b.layer.Forward(b.operator.Resolve(b.ForwardInID))

	if b.EnableBackward {
		b.layer.Backward(b.operator.Resolve(b.BackwardInID))
	}
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
