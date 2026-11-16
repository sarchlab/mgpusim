package cfd

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
	N         int
	// Host data (deterministic, reproducible in Verify).
	HRho       []float32
	HMx        []float32
	HMy        []float32
	HMz        []float32
	HEnergy    []float32
	HNeighbors []int32
	HNormals   []float32
	HAreas     []float32
	// Device buffers.
	GRho             gpuruntime.Ptr
	GMx              gpuruntime.Ptr
	GMy              gpuruntime.Ptr
	GMz              gpuruntime.Ptr
	GEnergy          gpuruntime.Ptr
	GNeighbors       gpuruntime.Ptr
	GNormals         gpuruntime.Ptr
	GAreas           gpuruntime.Ptr
	GFluxRho         gpuruntime.Ptr
	GFluxMx          gpuruntime.Ptr
	GFluxMy          gpuruntime.Ptr
	GFluxMz          gpuruntime.Ptr
	GFluxEnergy      gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
