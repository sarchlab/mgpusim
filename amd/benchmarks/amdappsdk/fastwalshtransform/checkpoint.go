package fastwalshtransform

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
	Stage            uint32
	QueueIndex       int
	Readback         driver.CommandID
	Context          driver.Context
	Gpus             []int
	Queues           []driver.CommandQueue
	Arch             arch.Type
	Length           uint32
	HInputArray      []float32
	HVerInputArray   []float32
	DInputArray      driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	Length           uint32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		Length:           b.Length,
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
		Stage:            b.stage,
		QueueIndex:       b.queueIndex,
		Readback:         b.readback,
		Context:          b.context,
		Gpus:             b.gpus,
		Queues:           b.queues,
		Arch:             b.Arch,
		Length:           b.Length,
		HInputArray:      b.hInputArray,
		HVerInputArray:   b.hVerInputArray,
		DInputArray:      b.dInputArray,
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
	b.stage = s.Stage
	b.queueIndex = s.QueueIndex
	b.readback = s.Readback
	b.context = s.Context
	b.gpus = s.Gpus
	b.queues = s.Queues
	b.Arch = s.Arch
	b.Length = s.Length
	b.hInputArray = s.HInputArray
	b.hVerInputArray = s.HVerInputArray
	b.dInputArray = s.DInputArray
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadProgram()
	return nil
}
