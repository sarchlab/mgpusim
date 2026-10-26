package gramschmidt

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress
	Readbacks        []driver.CommandID
	Context          driver.Context
	Queue            driver.CommandQueue
	Gpus             []int
	Arch             arch.Type
	M                int
	N                int
	AInit            []float32 // original A, M x N (row-major)
	GA               driver.Ptr
	GQ               driver.Ptr
	GR               driver.Ptr
	GNrm             driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	M                int
	N                int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		M:                b.M,
		N:                b.N,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
