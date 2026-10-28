package nw

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress

	Readbacks         []benchmarks.Readback
	Context           gpuruntime.Context
	GpuIDs            []int
	UseUnifiedMemory  bool
	Queue             gpuruntime.CommandQueue
	Arch              arch.Type
	BlockSize         int
	Length            int
	Penalty           int
	Row               int
	Col               int
	Reference         []int32
	InputItemSets     []int32
	OutputItemSets    []int32
	CpuOutputItemSets []int32
	DInputItemSets    gpuruntime.Ptr
	DOutputItemSets   gpuruntime.Ptr
	DReference        gpuruntime.Ptr
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
