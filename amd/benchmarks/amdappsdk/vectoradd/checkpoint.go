package vectoradd

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"io"
)

type checkpointState struct {
	Progress         benchmarks.Progress
	InitialConfig    checkpointConfig
	ConfigCaptured   bool
	Readbacks        []benchmarks.Readback
	Queue            driver.CommandQueue
	Context          driver.Context
	Gpus             []int
	Width            uint32
	Height           uint32
	HA               []float32
	HB               []float32
	HC               []float32
	DA               driver.Ptr
	DB               driver.Ptr
	DC               driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Width            uint32
	Height           uint32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Width:            b.Width,
		Height:           b.Height,
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
		Readbacks:        b.readbacks,
		Queue:            b.queue,
		Context:          b.context,
		Gpus:             b.gpus,
		Width:            b.Width,
		Height:           b.Height,
		HA:               b.hA,
		HB:               b.hB,
		HC:               b.hC,
		DA:               b.dA,
		DB:               b.dB,
		DC:               b.dC,
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
	b.readbacks = s.Readbacks
	b.queue = s.Queue
	b.context = s.Context
	b.gpus = s.Gpus
	b.Width = s.Width
	b.Height = s.Height
	b.hA = s.HA
	b.hB = s.HB
	b.hC = s.HC
	b.dA = s.DA
	b.dB = s.DB
	b.dC = s.DC
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadProgram()
	return nil
}
