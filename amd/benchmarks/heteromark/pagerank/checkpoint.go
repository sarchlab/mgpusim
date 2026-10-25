package pagerank

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/matrix/csr"
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
	Queues           []driver.CommandQueue
	Arch             arch.Type
	NumNodes         uint32
	NumConnections   uint32
	MaxIterations    uint32
	HMatrix          csr.Matrix
	HPageRank        []float32
	VerPageRank      []float32
	VerPageRankTemp  []float32
	DPageRank        driver.Ptr
	DPageRankTemp    driver.Ptr
	DRowOffsets      driver.Ptr
	DColumnNumbers   driver.Ptr
	DValues          driver.Ptr
	DLocalValues     driver.LocalPtr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NumNodes         uint32
	NumConnections   uint32
	MaxIterations    uint32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		NumNodes:         b.NumNodes,
		NumConnections:   b.NumConnections,
		MaxIterations:    b.MaxIterations,
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
		Queues:           b.queues,
		Arch:             b.Arch,
		NumNodes:         b.NumNodes,
		NumConnections:   b.NumConnections,
		MaxIterations:    b.MaxIterations,
		HMatrix:          b.hMatrix,
		HPageRank:        b.hPageRank,
		VerPageRank:      b.verPageRank,
		VerPageRankTemp:  b.verPageRankTemp,
		DPageRank:        b.dPageRank,
		DPageRankTemp:    b.dPageRankTemp,
		DRowOffsets:      b.dRowOffsets,
		DColumnNumbers:   b.dColumnNumbers,
		DValues:          b.dValues,
		DLocalValues:     b.dLocalValues,
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
	b.queues = s.Queues
	b.Arch = s.Arch
	b.NumNodes = s.NumNodes
	b.NumConnections = s.NumConnections
	b.MaxIterations = s.MaxIterations
	b.hMatrix = s.HMatrix
	b.hPageRank = s.HPageRank
	b.verPageRank = s.VerPageRank
	b.verPageRankTemp = s.VerPageRankTemp
	b.dPageRank = s.DPageRank
	b.dPageRankTemp = s.DPageRankTemp
	b.dRowOffsets = s.DRowOffsets
	b.dColumnNumbers = s.DColumnNumbers
	b.dValues = s.DValues
	b.dLocalValues = s.DLocalValues
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadProgram()
	return nil
}
