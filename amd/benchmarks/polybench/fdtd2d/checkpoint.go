package fdtd2d

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
	N                int // grid dimension (NX = NY = N)
	TMax             int // number of time steps
	ExInit           []float32
	EyInit           []float32
	HzInit           []float32
	GEx              gpuruntime.Ptr
	GEy              gpuruntime.Ptr
	GHz              gpuruntime.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	N                int
	TMax             int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		N:                b.N,
		TMax:             b.TMax,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
