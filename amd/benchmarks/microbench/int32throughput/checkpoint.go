package int32throughput

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
	// MadsPerThread is the number of multiply-add operations per thread.
	// It is rounded down to a multiple of 4 (and forced to at least 4),
	// matching the HIP host program.
	MadsPerThread int
	// NumBlocks is the number of work-groups (blocks) in the 1D grid.
	NumBlocks int
	// ThreadsPerBlock is the work-group (block) size, swept by the
	// calibration and passed to the kernel as an explicit argument.
	ThreadsPerBlock        int
	EffectiveMadsPerThread int
	NumThreads             int
	GOut                   gpuruntime.Ptr
	UseUnifiedMemory       bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	MadsPerThread    int
	NumBlocks        int
	ThreadsPerBlock  int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		MadsPerThread:    b.MadsPerThread,
		NumBlocks:        b.NumBlocks,
		ThreadsPerBlock:  b.ThreadsPerBlock,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
