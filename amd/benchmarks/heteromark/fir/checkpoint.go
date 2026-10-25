package fir

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"io"
)

type checkpointState struct {
	InitialConfig    checkpointConfig
	ConfigCaptured   bool
	Progress         benchmarks.Progress
	Readbacks        []driver.CommandID
	Context          driver.Context
	Queue            driver.CommandQueue
	Gpus             []int
	Arch             arch.Type
	Length           int
	NumTapsParam     int
	NumTaps          int
	InputData        []float32
	FilterData       []float32
	GFilterData      []driver.Ptr
	GHistoryData     driver.Ptr
	GInputData       driver.Ptr
	GOutputData      driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	Length           int
	NumTapsParam     int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		Length:           b.Length,
		NumTapsParam:     b.NumTapsParam,
		UseUnifiedMemory: b.useUnifiedMemory,
	}
}
func (b *Benchmark) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, b.configuration(), checkpointState{
		InitialConfig:    b.initialConfig,
		ConfigCaptured:   b.configCaptured,
		Progress:         b.Progress,
		Readbacks:        b.readbacks,
		Context:          b.context,
		Queue:            b.queue,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		Length:           b.Length,
		NumTapsParam:     b.NumTapsParam,
		NumTaps:          b.numTaps,
		InputData:        b.inputData,
		FilterData:       b.filterData,
		GFilterData:      b.gFilterData,
		GHistoryData:     b.gHistoryData,
		GInputData:       b.gInputData,
		GOutputData:      b.gOutputData,
		UseUnifiedMemory: b.useUnifiedMemory,
	}, nil)
}
func (b *Benchmark) LoadCheckpoint(r io.Reader) error {
	var s checkpointState
	if err := modeling.ReadCheckpoint(r, b.configuration(), &s, nil); err != nil {
		return err
	}
	b.initialConfig = s.InitialConfig
	b.configCaptured = s.ConfigCaptured
	b.Progress = s.Progress
	b.readbacks = s.Readbacks
	b.context = s.Context
	b.queue = s.Queue
	b.gpus = s.Gpus
	b.Arch = s.Arch
	b.Length = s.Length
	b.NumTapsParam = s.NumTapsParam
	b.numTaps = s.NumTaps
	b.inputData = s.InputData
	b.filterData = s.FilterData
	b.gFilterData = s.GFilterData
	b.gHistoryData = s.GHistoryData
	b.gInputData = s.GInputData
	b.gOutputData = s.GOutputData
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadProgram()
	return nil
}

func (b *Benchmark) configuration() checkpointConfig {
	if b.configCaptured {
		return b.initialConfig
	}
	return b.checkpointConfig()
}
