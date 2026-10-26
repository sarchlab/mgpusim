package lavamd

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress
	Readbacks []gpuruntime.HostBuffer
	Context   gpuruntime.Context
	Queue     gpuruntime.CommandQueue
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
	GPosX            gpuruntime.Ptr
	GPosY            gpuruntime.Ptr
	GPosZ            gpuruntime.Ptr
	GForceX          gpuruntime.Ptr
	GForceY          gpuruntime.Ptr
	GForceZ          gpuruntime.Ptr
	GEnergy          gpuruntime.Ptr
	GNeighborList    gpuruntime.Ptr
	GNeighborCount   gpuruntime.Ptr
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
