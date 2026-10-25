package matrixtranspose

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"io"
)

type checkpointState struct {
	Progress           benchmarks.Progress
	InitialConfig      checkpointConfig
	ConfigCaptured     bool
	Readbacks          []benchmarks.Readback
	Queue              driver.CommandQueue
	Context            driver.Context
	Gpus               []int
	Queues             []driver.CommandQueue
	Arch               arch.Type
	Width              int
	ElemsPerThread1Dim int
	BlockSize          int
	HInputData         []uint32
	HOutputData        []uint32
	DInputData         driver.Ptr
	DOutputData        driver.Ptr
	UseUnifiedMemory   bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	Width            int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		Width:            b.Width,
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
		Progress:           b.Progress,
		InitialConfig:      b.initialConfig,
		ConfigCaptured:     b.configCaptured,
		Readbacks:          b.readbacks,
		Queue:              b.queue,
		Context:            b.context,
		Gpus:               b.gpus,
		Queues:             b.queues,
		Arch:               b.Arch,
		Width:              b.Width,
		ElemsPerThread1Dim: b.elemsPerThread1Dim,
		BlockSize:          b.blockSize,
		HInputData:         b.hInputData,
		HOutputData:        b.hOutputData,
		DInputData:         b.dInputData,
		DOutputData:        b.dOutputData,
		UseUnifiedMemory:   b.useUnifiedMemory,
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
	b.queues = s.Queues
	b.Arch = s.Arch
	b.Width = s.Width
	b.elemsPerThread1Dim = s.ElemsPerThread1Dim
	b.blockSize = s.BlockSize
	b.hInputData = s.HInputData
	b.hOutputData = s.HOutputData
	b.dInputData = s.DInputData
	b.dOutputData = s.DOutputData
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadProgram()
	return nil
}
