package bitonicsort

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress

	Queues           []gpuruntime.CommandQueue
	Stage            int
	Pass             int
	NumStages        int
	Readback         gpuruntime.HostBuffer
	PerPassVerify    bool
	GpusToUse        []int
	Context          gpuruntime.Context
	Arch             arch.Type
	Length           int
	OrderAscending   bool
	InputData        []uint32
	OutputData       []uint32
	GInputData       gpuruntime.Ptr
	PerPassIn        []uint32
	PerPassOut       []uint32
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
