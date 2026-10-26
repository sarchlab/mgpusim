package nbody

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress

	Readbacks        []benchmarks.Readback
	Queue            driver.CommandQueue
	Context          driver.Context
	Gpus             []int
	UseUnifiedMemory bool
	Arch             arch.Type
	NumParticles     int32
	DelT             float32   // dT (timestep)
	EspSqr           float32   // Softening Factor
	InitPos          []float32 // initial position
	InitVel          []float32 // initial velocity
	Pos              []float32 // Output position
	Vel              []float32 // Output velocity
	RefPos           []float32 // Reference position
	RefVel           []float32 // Reference velocity
	GroupSize        int32     // Work-Group size
	NumIterations    int32
	Exchange         bool
	NumBodies        int32
	CurrPos          driver.Ptr
	CurrVel          driver.Ptr
	NewPos           driver.Ptr
	NewVel           driver.Ptr
	DPos             driver.Ptr
	DVel             driver.Ptr
	DNewPos          driver.Ptr
	DNewVel          driver.Ptr
	LocalPosBuf      driver.Ptr
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	UseUnifiedMemory bool
	Arch             arch.Type
	NumParticles     int32
	NumIterations    int32
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		UseUnifiedMemory: b.UseUnifiedMemory,
		Arch:             b.Arch,
		NumParticles:     b.NumParticles,
		NumIterations:    b.NumIterations,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
