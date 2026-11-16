package fastwalshtransform

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress

	Readback         gpuruntime.HostBuffer
	Context          gpuruntime.Context
	Gpus             []int
	Queues           []gpuruntime.CommandQueue
	Arch             arch.Type
	Length           uint32
	HInputArray      []float32
	HVerInputArray   []float32
	DInputArray      gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
