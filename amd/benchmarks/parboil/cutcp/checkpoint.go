package cutcp

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
	// NumAtoms is the number of atoms (point charges).
	NumAtoms int
	// GridSide is the number of grid points along each axis (grid is
	// GridSide^3 points).
	GridSide int
	// GridSpacing is the distance between adjacent grid points.
	GridSpacing float32
	// Cutoff is the interaction cutoff radius.
	Cutoff           float32
	Atoms            []float32 // flat float4: [x,y,z,q] per atom
	Potential        []float32 // GridSide^3 grid points
	GAtoms           gpuruntime.Ptr
	GPot             gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
