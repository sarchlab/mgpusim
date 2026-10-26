package fp16throughput

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress
	Readbacks []driver.HostBuffer
	Context   driver.Context
	Queue     driver.CommandQueue
	Gpus      []int
	Arch      arch.Type
	// FmasPerThread is the number of __hfma2 FMAs each thread performs
	// (rounded down to a multiple of 4). NumBlocks and ThreadsPerBlock
	// define the launch geometry.
	FmasPerThread    int
	NumBlocks        int
	ThreadsPerBlock  int
	GOut             driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	FmasPerThread    int
	NumBlocks        int
	ThreadsPerBlock  int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		FmasPerThread:    b.FmasPerThread,
		NumBlocks:        b.NumBlocks,
		ThreadsPerBlock:  b.ThreadsPerBlock,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
