package lbm

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress
	Readbacks        []driver.HostBuffer
	Context          driver.Context
	Queue            driver.CommandQueue
	Gpus             []int
	Arch             arch.Type
	GridDim          int       // N for the NxNxN grid
	NumTimesteps     int       // number of collide-stream iterations
	Tau              float32   // relaxation time; omega = 1/tau
	HF               []float32 // host initial distribution (Q*N)
	GFSrc            driver.Ptr
	GFDst            driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	GridDim          int
	NumTimesteps     int
	Tau              float32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		GridDim:          b.GridDim,
		NumTimesteps:     b.NumTimesteps,
		Tau:              b.Tau,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
