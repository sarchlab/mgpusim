package srad

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress
	Readbacks                           []gpuruntime.HostBuffer
	Context                             gpuruntime.Context
	Queue                               gpuruntime.CommandQueue
	Gpus                                []int
	Arch                                arch.Type
	ImageSize, NumIterations, Iteration int
	Lambda                              float32
	Initial, Current, Final             []float32
	Buffers                             [6]gpuruntime.Ptr
	UseUnifiedMemory                    bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
