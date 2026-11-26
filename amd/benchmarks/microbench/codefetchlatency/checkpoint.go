package codefetchlatency

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
	// BodyOps is the number of FMAs in the straight-line loop body. It selects
	// the kernel fetch_kernel_<BodyOps> and must be one of BodySizes.
	BodyOps int
	// Loops is how many times the kernel executes its body.
	Loops int
	// NumBlocks is the number of thread blocks to launch.
	NumBlocks int
	// ThreadsPerBlock is the work-group (block) size. Defaults to 64, one
	// wavefront; must be <= 1024 (the gfx942 hardware maximum).
	ThreadsPerBlock  int
	GOut             gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
