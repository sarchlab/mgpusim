package backprop

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress
	Readbacks []driver.HostBuffer
	Context   driver.Context
	Queue     driver.CommandQueue
	Gpus      []int
	Arch      arch.Type
	InputN    int
	HiddenN   int
	OutputN   int
	// Host-side reference inputs (deterministic, reproduced in Verify).
	HInput           []float32
	HW1              []float32
	HW2              []float32
	HB1              []float32
	HB2              []float32
	HTarget          []float32
	GInput           driver.Ptr
	GW1              driver.Ptr
	GB1              driver.Ptr
	GHidden          driver.Ptr
	GW2              driver.Ptr
	GB2              driver.Ptr
	GOutput          driver.Ptr
	GTarget          driver.Ptr
	GDeltaOut        driver.Ptr
	GDeltaHid        driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	InputN           int
	HiddenN          int
	OutputN          int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		InputN:           b.InputN,
		HiddenN:          b.HiddenN,
		OutputN:          b.OutputN,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
