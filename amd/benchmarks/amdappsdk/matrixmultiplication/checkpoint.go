package matrixmultiplication

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress

	Queue            gpuruntime.CommandQueue
	BlockABuf        gpuruntime.Ptr
	GA               gpuruntime.Ptr
	GB               gpuruntime.Ptr
	GC               gpuruntime.Ptr
	Readback         gpuruntime.HostBuffer
	Context          gpuruntime.Context
	Gpus             []int
	Arch             arch.Type
	X                uint32
	Y                uint32
	Z                uint32
	MatrixA          Matrix
	MatrixB          Matrix
	MatrixC          Matrix
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadKernel()
	return nil
}
