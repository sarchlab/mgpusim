package sharedmemlatency

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
	Readback         driver.CommandID
	Context          driver.Context
	Queue            driver.CommandQueue
	Gpus             []int
	Arch             arch.Type
	NumBlocks        int
	InnerIters       int
	BlockSize        int
	AccessPattern    string
	GSink            driver.Ptr
	NoConflictResult []float32
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NumBlocks        int
	InnerIters       int
	BlockSize        int
	AccessPattern    string
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		NumBlocks:        b.NumBlocks,
		InnerIters:       b.InnerIters,
		BlockSize:        b.BlockSize,
		AccessPattern:    b.AccessPattern,
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
		Readback:         b.readback,
		Context:          b.context,
		Queue:            b.queue,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		NumBlocks:        b.NumBlocks,
		InnerIters:       b.InnerIters,
		BlockSize:        b.BlockSize,
		AccessPattern:    b.AccessPattern,
		GSink:            b.gSink,
		NoConflictResult: b.noConflictResult,
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
	b.readback = s.Readback
	b.context = s.Context
	b.queue = s.Queue
	b.gpus = s.Gpus
	b.Arch = s.Arch
	b.NumBlocks = s.NumBlocks
	b.InnerIters = s.InnerIters
	b.BlockSize = s.BlockSize
	b.AccessPattern = s.AccessPattern
	b.gSink = s.GSink
	b.noConflictResult = s.NoConflictResult
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadProgram()
	return nil
}
