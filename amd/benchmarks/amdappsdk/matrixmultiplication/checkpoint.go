package matrixmultiplication

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress

	Queue            driver.CommandQueue
	BlockABuf        driver.Ptr
	GA               driver.Ptr
	GB               driver.Ptr
	GC               driver.Ptr
	Readback         driver.CommandID
	Context          driver.Context
	Gpus             []int
	Arch             arch.Type
	X                uint32
	Y                uint32
	Z                uint32
	MatrixA          Matrix
	MatrixB          Matrix
	MatrixC          Matrix
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	X                uint32
	Y                uint32
	Z                uint32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		X:                b.X,
		Y:                b.Y,
		Z:                b.Z,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadKernel()
	return nil
}
