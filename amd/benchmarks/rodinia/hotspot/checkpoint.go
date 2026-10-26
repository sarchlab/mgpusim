package hotspot

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress
	Readbacks     []driver.CommandID
	Context       driver.Context
	Queue         driver.CommandQueue
	Gpus          []int
	Arch          arch.Type
	GridSize      int       // grid is GridSize x GridSize
	NumIterations int       // stencil time-steps per run
	Temp          []float32 // initial temperature grid
	Power         []float32 // power density grid
	GTempA        driver.Ptr
	GTempB        driver.Ptr
	GPower        driver.Ptr
	// which buffer holds the final result after the iteration loop
	FinalSrc         driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	GridSize         int
	NumIterations    int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		GridSize:         b.GridSize,
		NumIterations:    b.NumIterations,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
