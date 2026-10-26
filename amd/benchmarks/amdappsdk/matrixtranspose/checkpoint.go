package matrixtranspose

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress

	Readbacks          []benchmarks.Readback
	Queue              gpuruntime.CommandQueue
	Context            gpuruntime.Context
	Gpus               []int
	Queues             []gpuruntime.CommandQueue
	Arch               arch.Type
	Width              int
	ElemsPerThread1Dim int
	BlockSize          int
	HInputData         []uint32
	HOutputData        []uint32
	DInputData         gpuruntime.Ptr
	DOutputData        gpuruntime.Ptr
	UseUnifiedMemory   bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	Width            int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		Width:            b.Width,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
