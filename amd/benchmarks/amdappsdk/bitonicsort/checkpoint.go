package bitonicsort

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"io"
)

type checkpointState struct {
	Progress         benchmarks.Progress
	InitialConfig    checkpointConfig
	ConfigCaptured   bool
	Queues           []driver.CommandQueue
	Stage            int
	Pass             int
	NumStages        int
	Readback         driver.CommandID
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
	return checkpointConfig{Verification: b.Verification, GPUs: b.gpusToUse,
		PerPassVerify:    b.PerPassVerify,
		Arch:             b.Arch,
		Length:           b.Length,
		OrderAscending:   b.OrderAscending,
		UseUnifiedMemory: b.useUnifiedMemory,
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
		Progress:         b.Progress,
		InitialConfig:    b.initialConfig,
		ConfigCaptured:   b.configCaptured,
		Queues:           b.queues,
		Stage:            b.stage,
		Pass:             b.pass,
		NumStages:        b.numStages,
		Readback:         b.readback,
		PerPassVerify:    b.PerPassVerify,
		GpusToUse:        b.gpusToUse,
		Context:          b.context,
		Arch:             b.Arch,
		Length:           b.Length,
		OrderAscending:   b.OrderAscending,
		InputData:        b.inputData,
		OutputData:       b.outputData,
		GInputData:       b.gInputData,
		PerPassIn:        b.perPassIn,
		PerPassOut:       b.perPassOut,
		UseUnifiedMemory: b.useUnifiedMemory,
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
	b.queues = s.Queues
	b.stage = s.Stage
	b.pass = s.Pass
	b.numStages = s.NumStages
	b.readback = s.Readback
	b.PerPassVerify = s.PerPassVerify
	b.gpusToUse = s.GpusToUse
	b.context = s.Context
	b.Arch = s.Arch
	b.Length = s.Length
	b.OrderAscending = s.OrderAscending
	b.inputData = s.InputData
	b.outputData = s.OutputData
	b.gInputData = s.GInputData
	b.perPassIn = s.PerPassIn
	b.perPassOut = s.PerPassOut
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadProgram()
	return nil
}
