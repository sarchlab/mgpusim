package nw

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress

	Readbacks         []benchmarks.Readback
	Context           driver.Context
	GpuIDs            []int
	UseUnifiedMemory  bool
	Queue             driver.CommandQueue
	Arch              arch.Type
	BlockSize         int
	Length            int
	Penalty           int
	Row               int
	Col               int
	Reference         []int32
	InputItemSets     []int32
	OutputItemSets    []int32
	CpuOutputItemSets []int32
	DInputItemSets    driver.Ptr
	DOutputItemSets   driver.Ptr
	DReference        driver.Ptr
}
type checkpointConfig struct {
	GPUs             []int
	Length, Penalty  int
	Verification     bool
	UseUnifiedMemory bool
	Arch             arch.Type
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{GPUs: b.GpuIDs, Length: b.Length, Penalty: b.Penalty, Verification: b.Verification,
		UseUnifiedMemory: b.UseUnifiedMemory,
		Arch:             b.Arch,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
