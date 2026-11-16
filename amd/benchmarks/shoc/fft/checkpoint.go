package fft

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
	UseUnifiedMemory bool
	Arch             arch.Type
	Bytes            int64
	BytesMode        bool
	Passes           int32
	HalfNFfts        int64
	NFfts            int64
	HalfNCmplx       int64
	UsedBytes        uint64
	DSource          gpuruntime.Ptr
	Source           []Float2
	Result           []Float2
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
