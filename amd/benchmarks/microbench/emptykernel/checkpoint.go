package emptykernel

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress

	Readback gpuruntime.HostBuffer
	Context  gpuruntime.Context
	Queue    gpuruntime.CommandQueue
	Gpus     []int
	Arch     arch.Type
	// NumBlocks is the number of work-groups in the (empty) grid; the scaling axis.
	NumBlocks int
	// BlockSize is the number of work-items per work-group.
	BlockSize        int
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
