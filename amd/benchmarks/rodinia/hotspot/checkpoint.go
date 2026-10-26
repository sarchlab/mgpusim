package hotspot

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress
	Readbacks     []gpuruntime.HostBuffer
	Context       gpuruntime.Context
	Queue         gpuruntime.CommandQueue
	Gpus          []int
	Arch          arch.Type
	GridSize      int       // grid is GridSize x GridSize
	NumIterations int       // stencil time-steps per run
	Temp          []float32 // initial temperature grid
	Power         []float32 // power density grid
	GTempA        gpuruntime.Ptr
	GTempB        gpuruntime.Ptr
	GPower        gpuruntime.Ptr
	// which buffer holds the final result after the iteration loop
	FinalSrc         gpuruntime.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	GridSize         int
	NumIterations    int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		GridSize:         b.GridSize,
		NumIterations:    b.NumIterations,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
