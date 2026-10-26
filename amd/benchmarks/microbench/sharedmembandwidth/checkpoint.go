package sharedmembandwidth

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress

	Readback driver.CommandID
	Context  driver.Context
	Queue    driver.CommandQueue
	Gpus     []int
	Arch     arch.Type
	// NumBlocks is the number of work-groups (grid size in the X dimension).
	NumBlocks int
	// InnerIters is the number of streaming iterations the kernel runs.
	InnerIters int
	// BlockSize is the work-group size (threads per block), swept by the
	// calibration and passed to the kernel as an explicit argument.
	BlockSize int
	// AccessPattern selects which kernel(s) run: "no_conflict" (default),
	// "conflict", or "both".
	AccessPattern string
	GIn           driver.Ptr
	GSink         driver.Ptr
	// noConflictResult holds the d_sink contents after the no-conflict
	// kernel, snapshotted before the conflict kernel overwrites the sink.
	NoConflictResult []float32
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NumBlocks        int
	InnerIters       int
	BlockSize        int
	AccessPattern    string
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		NumBlocks:        b.NumBlocks,
		InnerIters:       b.InnerIters,
		BlockSize:        b.BlockSize,
		AccessPattern:    b.AccessPattern,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
