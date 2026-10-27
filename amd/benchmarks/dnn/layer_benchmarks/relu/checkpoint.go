package relu

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress

	Readbacks        []benchmarks.Readback
	Queue            gpuruntime.CommandQueue
	Context          gpuruntime.Context
	Gpus             []int
	Arch             arch.Type
	Length           int
	InputData        []float32
	OutputData       []float32
	GInputData       gpuruntime.Ptr
	GOutputData      gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
