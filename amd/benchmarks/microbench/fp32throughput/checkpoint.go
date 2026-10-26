package fp32throughput

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress
	Readbacks []driver.CommandID
	Context   driver.Context
	Queue     driver.CommandQueue
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
	GOut             driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NumBlocks        int
	FmasPerThread    int
	ThreadsPerBlock  int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		NumBlocks:        b.NumBlocks,
		FmasPerThread:    b.FmasPerThread,
		ThreadsPerBlock:  b.ThreadsPerBlock,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
