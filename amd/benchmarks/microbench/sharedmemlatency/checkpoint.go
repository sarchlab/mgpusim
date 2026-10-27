package sharedmemlatency

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
	// NumBlocks is the number of work-groups (grid size in the X dimension).
	NumBlocks int
	// InnerIters is the number of outer timing iterations the kernel runs.
	InnerIters int
	// BlockSize is the work-group size (threads per block). Swept by the
	// calibration and passed to the kernel as an explicit argument.
	BlockSize int
	// AccessPattern selects which kernel(s) run: "no_conflict" (default),
	// "conflict", or "both". The HW sweep measures one pattern per config.
	AccessPattern string
	GSink         gpuruntime.Ptr
	// noConflictResult holds the d_sink contents after the no-conflict
	// kernel, snapshotted before the conflict kernel overwrites the sink.
	// It is nil when the no-conflict kernel was not run.
	NoConflictResult []float32
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
