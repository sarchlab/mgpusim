package cutcp

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
	GAtoms           driver.Ptr
	GPot             driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NumAtoms         int
	GridSide         int
	GridSpacing      float32
	Cutoff           float32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		NumAtoms:         b.NumAtoms,
		GridSide:         b.GridSide,
		GridSpacing:      b.GridSpacing,
		Cutoff:           b.Cutoff,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
