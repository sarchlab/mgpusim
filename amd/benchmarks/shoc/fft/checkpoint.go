package fft

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress

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
		Gpus:             b.Gpus,
		UseUnifiedMemory: b.UseUnifiedMemory,
		Arch:             b.Arch,
		Bytes:            b.Bytes,
		BytesMode:        b.BytesMode,
		Passes:           b.Passes,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
