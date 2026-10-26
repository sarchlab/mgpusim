package aes

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress

	Queue            driver.CommandQueue
	Readback         driver.CommandID
	Context          driver.Context
	Gpus             []int
	Arch             arch.Type
	Length           int
	Input            []byte
	Key              []byte
	ExpandedKey      []uint32
	S                []byte
	GInput           driver.Ptr
	GExpandedKey     []driver.Ptr
	GS               []driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	Length           int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		Length:           b.Length,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
