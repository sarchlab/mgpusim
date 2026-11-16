package kmeans

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress

	Iteration        int
	Readback         gpuruntime.HostBuffer
	SwapReadback     gpuruntime.HostBuffer
	Context          gpuruntime.Context
	Gpus             []int
	Queues           []gpuruntime.CommandQueue
	Arch             arch.Type
	NumClusters      int
	NumPoints        int
	NumFeatures      int
	MaxIter          int
	HFeatures        []float32
	DFeatures        gpuruntime.Ptr
	DFeaturesSwap    gpuruntime.Ptr
	HMembership      []int32
	DMembership      gpuruntime.Ptr
	HClusters        []float32
	DClusters        []gpuruntime.Ptr
	GpuRMSE          float64
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadKernels()
	return nil
}
