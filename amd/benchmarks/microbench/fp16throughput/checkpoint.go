package fp16throughput

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
	// FmasPerThread is the number of __hfma2 FMAs each thread performs
	// (rounded down to a multiple of 4). NumBlocks and ThreadsPerBlock
	// define the launch geometry.
	FmasPerThread    int
	NumBlocks        int
	ThreadsPerBlock  int
	GOut             gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
