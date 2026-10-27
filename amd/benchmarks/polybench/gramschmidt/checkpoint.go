package gramschmidt

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress
	Readbacks        []gpuruntime.HostBuffer
	Context          gpuruntime.Context
	Queue            gpuruntime.CommandQueue
	Gpus             []int
	Arch             arch.Type
	M                int
	N                int
	AInit            []float32 // original A, M x N (row-major)
	GA               gpuruntime.Ptr
	GQ               gpuruntime.Ptr
	GR               gpuruntime.Ptr
	GNrm             gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
