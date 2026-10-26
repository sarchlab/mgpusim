package simpleconvolution

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
	MaskSize         uint32
	Gpus             []int
	Arch             arch.Type
	Width            uint32
	Height           uint32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification, MaskSize: b.MaskSize,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		Width:            b.Width,
		Height:           b.Height,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
