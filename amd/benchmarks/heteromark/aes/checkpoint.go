package aes

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress

	Queue            gpuruntime.CommandQueue
	Readback         gpuruntime.HostBuffer
	Context          gpuruntime.Context
	Gpus             []int
	Arch             arch.Type
	Length           int
	Input            []byte
	Key              []byte
	ExpandedKey      []uint32
	S                []byte
	GInput           gpuruntime.Ptr
	GExpandedKey     []gpuruntime.Ptr
	GS               []gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
