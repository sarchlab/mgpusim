package lavamd

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
	// NumBoxes is the number of boxes per dimension (the grid is
	// NumBoxes^3 boxes).
	NumBoxes int
	// ParticlesPerBox is the number of particles in each box.
	ParticlesPerBox  int
	TotalBoxes       int
	TotalParticles   int
	HPosX            []float32
	HPosY            []float32
	HPosZ            []float32
	HNeighborList    []int32
	HNeighborCount   []int32
	GPosX            driver.Ptr
	GPosY            driver.Ptr
	GPosZ            driver.Ptr
	GForceX          driver.Ptr
	GForceY          driver.Ptr
	GForceZ          driver.Ptr
	GEnergy          driver.Ptr
	GNeighborList    driver.Ptr
	GNeighborCount   driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NumBoxes         int
	ParticlesPerBox  int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		NumBoxes:         b.NumBoxes,
		ParticlesPerBox:  b.ParticlesPerBox,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
