package stencil2d

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
	WCenter          float32
	WCardinal        float32
	WDiagonal        float32
	HInput           []float32
	HOutput          []float32
	NumIteration     int
	HaloWidth        int
	DData1           gpuruntime.Ptr
	DData2           gpuruntime.Ptr
	CurrData         gpuruntime.Ptr
	NewData          gpuruntime.Ptr
	NumRows          int
	NumCols          int
	DataSize         int
	NumPaddedCols    int
	PaddedDataSize   int
	Pad              int
	LocalRows        int
	LocalCols        int
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadKernels()
	return nil
}
