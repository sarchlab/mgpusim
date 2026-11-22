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
	NumBoxes                              int
	ParticlesPerBox                       int
	Dim                                   Dimensions
	Positions                             []Vector
	Charges                               []float64
	GBoxes, GPositions, GCharges, GForces gpuruntime.Ptr
	UseUnifiedMemory                      bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
