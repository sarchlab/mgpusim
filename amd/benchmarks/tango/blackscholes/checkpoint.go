package blackscholes

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
	N                int
	HS               []float32
	HK               []float32
	HT               []float32
	HSigma           []float32
	GS               driver.Ptr
	GK               driver.Ptr
	GT               driver.Ptr
	GSigma           driver.Ptr
	GCall            driver.Ptr
	GPut             driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	N                int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		N:                b.N,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
