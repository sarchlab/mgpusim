package conv2d

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
	N                int
	A                []float32
	GA               gpuruntime.Ptr
	GB               gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
