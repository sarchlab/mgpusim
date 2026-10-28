package lbm

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress
	Readbacks        []gpuruntime.HostBuffer
	Context          gpuruntime.Context
	Queue            gpuruntime.CommandQueue
	Gpus             []int
	Arch             arch.Type
	GridDim          int       // N for the NxNxN grid
	NumTimesteps     int       // number of collide-stream iterations
	Tau              float32   // relaxation time; omega = 1/tau
	HF               []float32 // host initial distribution (Q*N)
	GFSrc            gpuruntime.Ptr
	GFDst            gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
