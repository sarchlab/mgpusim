package fir

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
	Length           int
	NumTapsParam     int
	NumTaps          int
	InputData        []float32
	FilterData       []float32
	GFilterData      []gpuruntime.Ptr
	GHistoryData     gpuruntime.Ptr
	GInputData       gpuruntime.Ptr
	GOutputData      gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
