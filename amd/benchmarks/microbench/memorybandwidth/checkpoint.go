package memorybandwidth

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress
	Readbacks []driver.CommandID
	Context   driver.Context
	Queue     driver.CommandQueue
	Gpus      []int
	Arch      arch.Type
	// NumElements is the number of float32 elements copied device-to-device.
	NumElements      int
	Src              []float32
	GSrc             driver.Ptr
	GDst             driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NumElements      int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		NumElements:      b.NumElements,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
