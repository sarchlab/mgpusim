package emptykernel

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
	// NumBlocks is the number of work-groups in the (empty) grid; the scaling axis.
	NumBlocks int
	// BlockSize is the number of work-items per work-group.
	BlockSize        int
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NumBlocks        int
	BlockSize        int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		NumBlocks:        b.NumBlocks,
		BlockSize:        b.BlockSize,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
