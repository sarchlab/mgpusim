package nw

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"io"
)

type checkpointState struct {
	Progress          benchmarks.Progress
	InitialConfig     checkpointConfig
	ConfigCaptured    bool
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
	return checkpointConfig{GPUs: b.gpuIDs, Length: b.length, Penalty: b.penalty, Verification: b.Verification,
		UseUnifiedMemory: b.useUnifiedMemory,
		Arch:             b.Arch,
	}
}
func (b *Benchmark) configuration() checkpointConfig {
	if b.configCaptured {
		return b.initialConfig
	}
	return b.checkpointConfig()
}
func (b *Benchmark) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, b.configuration(), checkpointState{
		Progress:          b.Progress,
		InitialConfig:     b.initialConfig,
		ConfigCaptured:    b.configCaptured,
		Readbacks:         b.readbacks,
		Context:           b.context,
		GpuIDs:            b.gpuIDs,
		UseUnifiedMemory:  b.useUnifiedMemory,
		Queue:             b.queue,
		Arch:              b.Arch,
		BlockSize:         b.blockSize,
		Length:            b.length,
		Penalty:           b.penalty,
		Row:               b.row,
		Col:               b.col,
		Reference:         b.reference,
		InputItemSets:     b.inputItemSets,
		OutputItemSets:    b.outputItemSets,
		CpuOutputItemSets: b.cpuOutputItemSets,
		DInputItemSets:    b.dInputItemSets,
		DOutputItemSets:   b.dOutputItemSets,
		DReference:        b.dReference,
	}, nil)
}
func (b *Benchmark) LoadCheckpoint(r io.Reader) error {
	var s checkpointState
	if err := modeling.ReadCheckpoint(r, b.configuration(), &s, nil); err != nil {
		return err
	}
	b.Progress = s.Progress
	b.initialConfig = s.InitialConfig
	b.configCaptured = s.ConfigCaptured
	b.readbacks = s.Readbacks
	b.context = s.Context
	b.gpuIDs = s.GpuIDs
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.queue = s.Queue
	b.Arch = s.Arch
	b.blockSize = s.BlockSize
	b.length = s.Length
	b.penalty = s.Penalty
	b.row = s.Row
	b.col = s.Col
	b.reference = s.Reference
	b.inputItemSets = s.InputItemSets
	b.outputItemSets = s.OutputItemSets
	b.cpuOutputItemSets = s.CpuOutputItemSets
	b.dInputItemSets = s.DInputItemSets
	b.dOutputItemSets = s.DOutputItemSets
	b.dReference = s.DReference
	b.loadProgram()
	return nil
}
