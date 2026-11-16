package hotspot3d

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress
	Readbacks []gpuruntime.HostBuffer
	Context   gpuruntime.Context
	Queue     gpuruntime.CommandQueue
	Gpus      []int
	Arch      arch.Type
	// GridSize is the edge length N of the NxNxN grid.
	GridSize int
	// NumIterations is the number of stencil time-steps to run.
	NumIterations int
	// AmbTemp is the ambient temperature.
	AmbTemp float32
	// Derived thermal parameters (computed in initMem).
	StepDivCap float32
	Rx1        float32
	Ry1        float32
	Rz1        float32
	Ra1        float32
	TempInit   []float32
	Power      []float32
	// Two device temperature buffers (ping-pong) plus the power buffer.
	GTemp  [2]gpuruntime.Ptr
	GPower gpuruntime.Ptr
	// finalBuf is the index of the device buffer holding the final result.
	FinalBuf         int
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
