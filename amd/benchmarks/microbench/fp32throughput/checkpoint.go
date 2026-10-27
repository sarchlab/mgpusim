package fp32throughput

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
	// NumBlocks is the number of thread blocks to launch.
	NumBlocks int
	// FmasPerThread is the number of FMA iterations per thread. It is rounded
	// down to a multiple of 4 to match the kernel's 4-way unrolled loop.
	FmasPerThread int
	// ThreadsPerBlock is the work-group (block) size. Defaults to 256; must be
	// <= 1024 (the gfx942 hardware maximum). The kernel reads blockDim.x, so any
	// value launches correctly and the sweep can match the ground-truth
	// threads_per_block dimension.
	ThreadsPerBlock  int
	GOut             gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
