package bfs

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress

	Depth            int
	FlagResult       gpuruntime.HostBuffer
	OutputResult     gpuruntime.HostBuffer
	Queue            gpuruntime.CommandQueue
	Context          gpuruntime.Context
	Gpus             []int
	Queues           []gpuruntime.CommandQueue
	Arch             arch.Type
	Path             string
	NumNode          int
	Degree           int
	MaxDepth         int
	SourceNode       int
	HEdgeOffsets     []int32
	HEdgeList        []int32
	HFrontier        []uint32
	HCost            []uint32
	CpuCost          []uint32
	DFrontier        gpuruntime.Ptr
	DEdgeArray       gpuruntime.Ptr
	DEdgeArrayAux    gpuruntime.Ptr
	DFlag            gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
