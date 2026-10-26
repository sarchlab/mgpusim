package bitonicsort

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress

	Queues           []driver.CommandQueue
	Stage            int
	Pass             int
	NumStages        int
	Readback         driver.HostBuffer
	PerPassVerify    bool
	GpusToUse        []int
	Context          driver.Context
	Arch             arch.Type
	Length           int
	OrderAscending   bool
	InputData        []uint32
	OutputData       []uint32
	GInputData       driver.Ptr
	PerPassIn        []uint32
	PerPassOut       []uint32
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	GPUs             []int
	PerPassVerify    bool
	Arch             arch.Type
	Length           int
	OrderAscending   bool
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification, GPUs: b.GpusToUse,
		PerPassVerify:    b.PerPassVerify,
		Arch:             b.Arch,
		Length:           b.Length,
		OrderAscending:   b.OrderAscending,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
