package backprop

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress
	Readbacks []gpuruntime.HostBuffer
	Context   gpuruntime.Context
	Queue     gpuruntime.CommandQueue
	Gpus      []int
	Arch      arch.Type
	InputN    int
	HiddenN   int
	OutputN   int
	// Host-side reference inputs (deterministic, reproduced in Verify).
	HInput           []float32
	HW1              []float32
	HW2              []float32
	HB1              []float32
	HB2              []float32
	HTarget          []float32
	GInput           gpuruntime.Ptr
	GW1              gpuruntime.Ptr
	GB1              gpuruntime.Ptr
	GHidden          gpuruntime.Ptr
	GW2              gpuruntime.Ptr
	GB2              gpuruntime.Ptr
	GOutput          gpuruntime.Ptr
	GTarget          gpuruntime.Ptr
	GDeltaOut        gpuruntime.Ptr
	GDeltaHid        gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
