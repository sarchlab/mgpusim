package matrixtranspose

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress

	Readbacks          []benchmarks.Readback
	Queue              driver.CommandQueue
	Context            driver.Context
	Gpus               []int
	Queues             []driver.CommandQueue
	Arch               arch.Type
	Width              int
	ElemsPerThread1Dim int
	BlockSize          int
	HInputData         []uint32
	HOutputData        []uint32
	DInputData         driver.Ptr
	DOutputData        driver.Ptr
	UseUnifiedMemory   bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	Width            int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		Width:            b.Width,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
