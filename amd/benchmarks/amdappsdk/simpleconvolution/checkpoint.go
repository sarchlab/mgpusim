package simpleconvolution

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
	Arch             arch.Type
	Width            uint32
	Height           uint32
	MaskSize         uint32
	PadWidth         uint32
	PadHeight        uint32
	HInputData       []uint32
	HOutputData      []uint32
	HMask            []float32
	DInputData       driver.Ptr
	DOutputData      driver.Ptr
	DMasks           []driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	Width            uint32
	Height           uint32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		Width:            b.Width,
		Height:           b.Height,
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
		Arch:             b.Arch,
		Width:            b.Width,
		Height:           b.Height,
		MaskSize:         b.maskSize,
		PadWidth:         b.padWidth,
		PadHeight:        b.padHeight,
		HInputData:       b.hInputData,
		HOutputData:      b.hOutputData,
		HMask:            b.hMask,
		DInputData:       b.dInputData,
		DOutputData:      b.dOutputData,
		DMasks:           b.dMasks,
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
	b.Arch = s.Arch
	b.Width = s.Width
	b.Height = s.Height
	b.maskSize = s.MaskSize
	b.padWidth = s.PadWidth
	b.padHeight = s.PadHeight
	b.hInputData = s.HInputData
	b.hOutputData = s.HOutputData
	b.hMask = s.HMask
	b.dInputData = s.DInputData
	b.dOutputData = s.DOutputData
	b.dMasks = s.DMasks
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadProgram()
	return nil
}
