package cfd

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress
	Readbacks                                                  []gpuruntime.HostBuffer
	Context                                                    gpuruntime.Context
	Queue                                                      gpuruntime.CommandQueue
	Gpus                                                       []int
	Arch                                                       arch.Type
	N, BlockSize, Padded                                       int
	Initial, Areas, Normals                                    []float32
	Neighbors                                                  []int32
	GState, GOld, GFlux, GFactor, GAreas, GNormals, GNeighbors gpuruntime.Ptr
	UseUnifiedMemory                                           bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
