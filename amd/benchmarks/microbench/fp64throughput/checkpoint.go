package fp64throughput

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
	// NumBlocks is the number of work-groups (grid dim in blocks).
	NumBlocks int
	// ThreadsPerBlock is the work-group (block) size, swept by the
	// calibration and passed to the kernel as an explicit argument.
	ThreadsPerBlock int
	// FmasPerThread is the number of FMA iterations per work-item. It is
	// rounded down to a multiple of 4 (matching the host program), with a
	// minimum of 4. Each iteration performs 4 FP64 FMAs (one per
	// accumulator), so total FP64 FLOPs = numThreads * FmasPerThread * 2.
	FmasPerThread    int
	NumThreads       int
	In               []float64
	GIn              gpuruntime.Ptr
	GOut             gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
