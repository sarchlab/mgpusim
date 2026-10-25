package cachelatency

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
	ArrayBytes       int
	CachelineBytes   int
	MeasureLaps      int
	NumAccesses      int
	Seed             uint32
	N                int
	M                int
	Stride           int
	StartIdx         uint32
	Chain            []uint32
	GArr             driver.Ptr
	GResult          driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	ArrayBytes       int
	CachelineBytes   int
	MeasureLaps      int
	NumAccesses      int
	Seed             uint32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		ArrayBytes:       b.ArrayBytes,
		CachelineBytes:   b.CachelineBytes,
		MeasureLaps:      b.MeasureLaps,
		NumAccesses:      b.NumAccesses,
		Seed:             b.Seed,
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
		ArrayBytes:       b.ArrayBytes,
		CachelineBytes:   b.CachelineBytes,
		MeasureLaps:      b.MeasureLaps,
		NumAccesses:      b.NumAccesses,
		Seed:             b.Seed,
		N:                b.n,
		M:                b.m,
		Stride:           b.stride,
		StartIdx:         b.startIdx,
		Chain:            b.chain,
		GArr:             b.gArr,
		GResult:          b.gResult,
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
	b.ArrayBytes = s.ArrayBytes
	b.CachelineBytes = s.CachelineBytes
	b.MeasureLaps = s.MeasureLaps
	b.NumAccesses = s.NumAccesses
	b.Seed = s.Seed
	b.n = s.N
	b.m = s.M
	b.stride = s.Stride
	b.startIdx = s.StartIdx
	b.chain = s.Chain
	b.gArr = s.GArr
	b.gResult = s.GResult
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
