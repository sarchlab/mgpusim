package aes

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
		Queue:            b.queue,
		Readback:         b.readback,
		Context:          b.context,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		Length:           b.Length,
		Input:            b.input,
		Key:              b.key,
		ExpandedKey:      b.expandedKey,
		S:                b.s,
		GInput:           b.gInput,
		GExpandedKey:     b.gExpandedKey,
		GS:               b.gS,
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
	b.queue = s.Queue
	b.readback = s.Readback
	b.context = s.Context
	b.gpus = s.Gpus
	b.Arch = s.Arch
	b.Length = s.Length
	b.input = s.Input
	b.key = s.Key
	b.expandedKey = s.ExpandedKey
	b.s = s.S
	b.gInput = s.GInput
	b.gExpandedKey = s.GExpandedKey
	b.gS = s.GS
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadProgram()
	return nil
}
