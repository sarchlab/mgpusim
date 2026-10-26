package hotspot3d

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
	// GridSize is the edge length N of the NxNxN grid.
	GridSize int
	// NumIterations is the number of stencil time-steps to run.
	NumIterations int
	// AmbTemp is the ambient temperature.
	AmbTemp float32
	// Derived thermal parameters (computed in initMem).
	StepDivCap float32
	Rx1        float32
	Ry1        float32
	Rz1        float32
	Ra1        float32
	TempInit   []float32
	Power      []float32
	// Two device temperature buffers (ping-pong) plus the power buffer.
	GTemp  [2]driver.Ptr
	GPower driver.Ptr
	// finalBuf is the index of the device buffer holding the final result.
	FinalBuf         int
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	GridSize         int
	NumIterations    int
	AmbTemp          float32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		GridSize:         b.GridSize,
		NumIterations:    b.NumIterations,
		AmbTemp:          b.AmbTemp,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
