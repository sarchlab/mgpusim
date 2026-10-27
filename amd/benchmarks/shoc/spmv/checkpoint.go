package spmv

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/matrix/csr"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress

	Readbacks        []benchmarks.Readback
	Queue            gpuruntime.CommandQueue
	Context          gpuruntime.Context
	Gpus             []int
	UseUnifiedMemory bool
	Arch             arch.Type
	Dim              int32
	Sparsity         float64
	DValData         gpuruntime.Ptr
	DVecData         gpuruntime.Ptr
	DColsData        gpuruntime.Ptr
	DRowDData        gpuruntime.Ptr
	DOutData         gpuruntime.Ptr
	NItems           int32
	Vec              []float32
	Out              []float32
	Maxval           float32
	Matrix           csr.Matrix
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
