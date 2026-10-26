package vectoradd

import (
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress

	Readbacks        []benchmarks.Readback
	Queue            driver.CommandQueue
	Context          driver.Context
	Gpus             []int
	Width            uint32
	Height           uint32
	HA               []float32
	HB               []float32
	HC               []float32
	DA               driver.Ptr
	DB               driver.Ptr
	DC               driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Width            uint32
	Height           uint32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Width:            b.Width,
		Height:           b.Height,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
