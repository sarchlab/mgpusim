package backprop

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress
	Readbacks                                               []gpuruntime.HostBuffer
	Context                                                 gpuruntime.Context
	Queue                                                   gpuruntime.CommandQueue
	Gpus                                                    []int
	Arch                                                    arch.Type
	InputN                                                  int
	HiddenN                                                 int
	OutputN                                                 int
	Input, Weights, Partials                                []float32
	Hidden, HiddenDelta                                     [width]float32
	InitialOutputWeights, OutputWeights, OutputMomentum     [width]float32
	Output, OutputDelta                                     float32
	GInput, GHidden, GWeights, GPrevious, GDelta, GPartials gpuruntime.Ptr
	UseUnifiedMemory                                        bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
