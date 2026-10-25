package im2col

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"io"
)

type checkpointState struct {
	Operator    gputensor.OperatorState
	Initialized bool
	Input       gputensor.TensorState

	Progress         benchmarks.Progress
	InitialConfig    checkpointConfig
	ConfigCaptured   bool
	Context          driver.Context
	Gpus             []int
	UseUnifiedMemory bool
	Arch             arch.Type
	N                int
	C                int
	H                int
	W                int
	OutputH          int
	OutputW          int
	KernelHeight     int
	KernelWidth      int
	PadX             int
	PadY             int
	StrideX          int
	StrideY          int
	DilateX          int
	DilateY          int
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	UseUnifiedMemory bool
	Arch             arch.Type
	N                int
	C                int
	H                int
	W                int
	KernelHeight     int
	KernelWidth      int
	PadX             int
	PadY             int
	StrideX          int
	StrideY          int
	DilateX          int
	DilateY          int
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		UseUnifiedMemory: b.useUnifiedMemory,
		Arch:             b.Arch,
		N:                b.N,
		C:                b.C,
		H:                b.H,
		W:                b.W,
		KernelHeight:     b.KernelHeight,
		KernelWidth:      b.KernelWidth,
		PadX:             b.PadX,
		PadY:             b.PadY,
		StrideX:          b.StrideX,
		StrideY:          b.StrideY,
		DilateX:          b.DilateX,
		DilateY:          b.DilateY,
	}
}
func (b *Benchmark) configuration() checkpointConfig {
	if b.configCaptured {
		return b.initialConfig
	}
	return b.checkpointConfig()
}
func (b *Benchmark) SaveCheckpoint(w io.Writer) error {
	var operator gputensor.OperatorState
	initialized := b.operator != nil
	if initialized {
		operator = b.operator.Snapshot()
	}
	return modeling.WriteCheckpoint(w, b.configuration(), checkpointState{Operator: operator, Initialized: initialized,
		Input: gputensor.Snapshot(b.Input),

		Progress:         b.Progress,
		InitialConfig:    b.initialConfig,
		ConfigCaptured:   b.configCaptured,
		Context:          b.context,
		Gpus:             b.gpus,
		UseUnifiedMemory: b.useUnifiedMemory,
		Arch:             b.Arch,
		N:                b.N,
		C:                b.C,
		H:                b.H,
		W:                b.W,
		OutputH:          b.outputH,
		OutputW:          b.outputW,
		KernelHeight:     b.KernelHeight,
		KernelWidth:      b.KernelWidth,
		PadX:             b.PadX,
		PadY:             b.PadY,
		StrideX:          b.StrideX,
		StrideY:          b.StrideY,
		DilateX:          b.DilateX,
		DilateY:          b.DilateY,
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
	b.context = s.Context
	b.gpus = s.Gpus
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.Arch = s.Arch
	b.N = s.N
	b.C = s.C
	b.H = s.H
	b.W = s.W
	b.outputH = s.OutputH
	b.outputW = s.OutputW
	b.KernelHeight = s.KernelHeight
	b.KernelWidth = s.KernelWidth
	b.PadX = s.PadX
	b.PadY = s.PadY
	b.StrideX = s.StrideX
	b.StrideY = s.StrideY
	b.DilateX = s.DilateX
	b.DilateY = s.DilateY
	if s.Initialized {
		b.operator = gputensor.RestoreOperator(b.driver, s.Operator)
		b.Input = b.operator.RestoreTensor(s.Input)
	}
	return nil
}
