package stencil

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress
	Readbacks    []gpuruntime.HostBuffer
	Context      gpuruntime.Context
	Queue        gpuruntime.CommandQueue
	Gpus         []int
	Arch         arch.Type
	N            int // grid dimension (N*N*N)
	NumTimesteps int
	HData        []float32 // initial host data
	GA           gpuruntime.Ptr
	GB           gpuruntime.Ptr
	// finalSrc tracks which device buffer holds the final result after the
	// ping-pong iterations.
	FinalSrc         gpuruntime.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	N                int
	NumTimesteps     int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		N:                b.N,
		NumTimesteps:     b.NumTimesteps,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
