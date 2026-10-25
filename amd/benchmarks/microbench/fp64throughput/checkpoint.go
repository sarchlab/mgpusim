package fp64throughput

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
	NumBlocks        int
	ThreadsPerBlock  int
	FmasPerThread    int
	NumThreads       int
	In               []float64
	GIn              driver.Ptr
	GOut             driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NumBlocks        int
	ThreadsPerBlock  int
	FmasPerThread    int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		NumBlocks:        b.NumBlocks,
		ThreadsPerBlock:  b.ThreadsPerBlock,
		FmasPerThread:    b.FmasPerThread,
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
		NumBlocks:        b.NumBlocks,
		ThreadsPerBlock:  b.ThreadsPerBlock,
		FmasPerThread:    b.FmasPerThread,
		NumThreads:       b.numThreads,
		In:               b.in,
		GIn:              b.gIn,
		GOut:             b.gOut,
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
	b.NumBlocks = s.NumBlocks
	b.ThreadsPerBlock = s.ThreadsPerBlock
	b.FmasPerThread = s.FmasPerThread
	b.numThreads = s.NumThreads
	b.in = s.In
	b.gIn = s.GIn
	b.gOut = s.GOut
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
