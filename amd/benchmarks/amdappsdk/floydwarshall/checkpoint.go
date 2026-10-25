package floydwarshall

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"io"
)

type checkpointState struct {
	Progress                        benchmarks.Progress
	InitialConfig                   checkpointConfig
	ConfigCaptured                  bool
	Readbacks                       []benchmarks.Readback
	Queue                           driver.CommandQueue
	Context                         driver.Context
	Gpus                            []int
	Queues                          []driver.CommandQueue
	Arch                            arch.Type
	NumNodes                        uint32
	NumIterations                   uint32
	HOutputPathMatrix               []uint32
	HOutputPathDistanceMatrix       []uint32
	DOutputPathMatrix               driver.Ptr
	DOutputPathDistanceMatrix       driver.Ptr
	HVerificationPathMatrix         []uint32
	HVerificationPathDistanceMatrix []uint32
	UseUnifiedMemory                bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NumNodes         uint32
	NumIterations    uint32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		NumNodes:         b.NumNodes,
		NumIterations:    b.NumIterations,
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
		Progress:                        b.Progress,
		InitialConfig:                   b.initialConfig,
		ConfigCaptured:                  b.configCaptured,
		Readbacks:                       b.readbacks,
		Queue:                           b.queue,
		Context:                         b.context,
		Gpus:                            b.gpus,
		Queues:                          b.queues,
		Arch:                            b.Arch,
		NumNodes:                        b.NumNodes,
		NumIterations:                   b.NumIterations,
		HOutputPathMatrix:               b.hOutputPathMatrix,
		HOutputPathDistanceMatrix:       b.hOutputPathDistanceMatrix,
		DOutputPathMatrix:               b.dOutputPathMatrix,
		DOutputPathDistanceMatrix:       b.dOutputPathDistanceMatrix,
		HVerificationPathMatrix:         b.hVerificationPathMatrix,
		HVerificationPathDistanceMatrix: b.hVerificationPathDistanceMatrix,
		UseUnifiedMemory:                b.useUnifiedMemory,
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
	b.NumNodes = s.NumNodes
	b.NumIterations = s.NumIterations
	b.hOutputPathMatrix = s.HOutputPathMatrix
	b.hOutputPathDistanceMatrix = s.HOutputPathDistanceMatrix
	b.dOutputPathMatrix = s.DOutputPathMatrix
	b.dOutputPathDistanceMatrix = s.DOutputPathDistanceMatrix
	b.hVerificationPathMatrix = s.HVerificationPathMatrix
	b.hVerificationPathDistanceMatrix = s.HVerificationPathDistanceMatrix
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadProgram()
	return nil
}
