package babelstream

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
	Scalar           float32
	A                []float32
	B                []float32
	C                []float32
	GA               driver.Ptr
	GB               driver.Ptr
	GC               driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	N                int
	Scalar           float32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		N:                b.N,
		Scalar:           b.Scalar,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
