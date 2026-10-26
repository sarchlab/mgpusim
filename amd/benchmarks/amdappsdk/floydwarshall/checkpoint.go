package floydwarshall

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress

	Readbacks                       []benchmarks.Readback
	Queue                           gpuruntime.CommandQueue
	Context                         gpuruntime.Context
	Gpus                            []int
	Arch                            arch.Type
	NumNodes                        uint32
	NumIterations                   uint32
	HOutputPathMatrix               []uint32
	HOutputPathDistanceMatrix       []uint32
	DOutputPathMatrix               gpuruntime.Ptr
	DOutputPathDistanceMatrix       gpuruntime.Ptr
	HVerificationPathMatrix         []uint32
	HVerificationPathDistanceMatrix []uint32
	UseUnifiedMemory                bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NumNodes         uint32
	NumIterations    uint32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		NumNodes:         b.NumNodes,
		NumIterations:    b.NumIterations,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
