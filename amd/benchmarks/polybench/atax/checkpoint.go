package atax

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress

	Readbacks        []benchmarks.Readback
	Queue            gpuruntime.CommandQueue
	Context          gpuruntime.Context
	Gpus             []int
	Arch             arch.Type
	NX               int
	NY               int
	A                []float32
	X                []float32
	Y                []float32
	YOutput          []float32
	Tmp              []float32
	DA               gpuruntime.Ptr
	DX               gpuruntime.Ptr
	DY               gpuruntime.Ptr
	DTmp             gpuruntime.Ptr
	CpuY             []float32
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
