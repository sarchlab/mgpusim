package matrixtranspose

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress

	Readbacks          []benchmarks.Readback
	Queue              gpuruntime.CommandQueue
	Context            gpuruntime.Context
	Gpus               []int
	Queues             []gpuruntime.CommandQueue
	Arch               arch.Type
	Width              int
	ElemsPerThread1Dim int
	BlockSize          int
	HInputData         []uint32
	HOutputData        []uint32
	DInputData         gpuruntime.Ptr
	DOutputData        gpuruntime.Ptr
	UseUnifiedMemory   bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
