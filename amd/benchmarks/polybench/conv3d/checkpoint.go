package conv3d

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
	// N is the edge length of the NxNxN volume.
	N int
	// FilterSize is the edge length of the cubic filter (e.g. 3).
	FilterSize       int
	Input            []float32
	Filter           []float32
	GInput           gpuruntime.Ptr
	GFilter          gpuruntime.Ptr
	GOutput          gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
