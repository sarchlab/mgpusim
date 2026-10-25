package fft

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
	Readbacks        []benchmarks.Readback
	Queue            driver.CommandQueue
	Context          driver.Context
	Gpus             []int
	Queues           []driver.CommandQueue
	UseUnifiedMemory bool
	Arch             arch.Type
	Bytes            int64
	BytesMode        bool
	Passes           int32
	HalfNFfts        int64
	NFfts            int64
	HalfNCmplx       int64
	UsedBytes        uint64
	DSource          driver.Ptr
	Source           []Float2
	Result           []Float2
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	UseUnifiedMemory bool
	Arch             arch.Type
	Bytes            int64
	BytesMode        bool
	Passes           int32
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		UseUnifiedMemory: b.useUnifiedMemory,
		Arch:             b.Arch,
		Bytes:            b.Bytes,
		BytesMode:        b.BytesMode,
		Passes:           b.Passes,
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
		UseUnifiedMemory: b.useUnifiedMemory,
		Arch:             b.Arch,
		Bytes:            b.Bytes,
		BytesMode:        b.BytesMode,
		Passes:           b.Passes,
		HalfNFfts:        b.halfNFfts,
		NFfts:            b.nFfts,
		HalfNCmplx:       b.halfNCmplx,
		UsedBytes:        b.usedBytes,
		DSource:          b.dSource,
		Source:           b.source,
		Result:           b.result,
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
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.Arch = s.Arch
	b.Bytes = s.Bytes
	b.BytesMode = s.BytesMode
	b.Passes = s.Passes
	b.halfNFfts = s.HalfNFfts
	b.nFfts = s.NFfts
	b.halfNCmplx = s.HalfNCmplx
	b.usedBytes = s.UsedBytes
	b.dSource = s.DSource
	b.source = s.Source
	b.result = s.Result
	b.loadProgram()
	return nil
}
