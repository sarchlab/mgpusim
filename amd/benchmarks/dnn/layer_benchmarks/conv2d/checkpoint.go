package conv2d

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"io"
)

type checkpointState struct {
	Operator              gputensor.OperatorState
	Initialized           bool
	Layer                 layers.State
	ForwardIn, BackwardIn gputensor.TensorState

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
	KernelChannel    int
	KernelHeight     int
	KernelWidth      int
	PadX             int
	PadY             int
	StrideX          int
	StrideY          int
	EnableBackward   bool
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
	KernelChannel    int
	KernelHeight     int
	KernelWidth      int
	PadX             int
	PadY             int
	StrideX          int
	StrideY          int
	EnableBackward   bool
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
		KernelChannel:    b.KernelChannel,
		KernelHeight:     b.KernelHeight,
		KernelWidth:      b.KernelWidth,
		PadX:             b.PadX,
		PadY:             b.PadY,
		StrideX:          b.StrideX,
		StrideY:          b.StrideY,
		EnableBackward:   b.EnableBackward,
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
	var layer layers.State
	if b.layer != nil {
		var err error
		layer, err = layers.Snapshot(b.layer)
		if err != nil {
			return err
		}
	}
	return modeling.WriteCheckpoint(w, b.configuration(), checkpointState{Operator: operator, Initialized: initialized,
		Layer: layer, ForwardIn: gputensor.Snapshot(b.forwardIn), BackwardIn: gputensor.Snapshot(b.backwardIn),

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
		KernelChannel:    b.KernelChannel,
		KernelHeight:     b.KernelHeight,
		KernelWidth:      b.KernelWidth,
		PadX:             b.PadX,
		PadY:             b.PadY,
		StrideX:          b.StrideX,
		StrideY:          b.StrideY,
		EnableBackward:   b.EnableBackward,
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
	b.KernelChannel = s.KernelChannel
	b.KernelHeight = s.KernelHeight
	b.KernelWidth = s.KernelWidth
	b.PadX = s.PadX
	b.PadY = s.PadY
	b.StrideX = s.StrideX
	b.StrideY = s.StrideY
	b.EnableBackward = s.EnableBackward
	if s.Initialized {
		b.operator = gputensor.RestoreOperator(b.driver, s.Operator)
		b.forwardIn = b.operator.RestoreTensor(s.ForwardIn)
		b.backwardIn = b.operator.RestoreTensor(s.BackwardIn)
		l, err := layers.Restore(b.operator, s.Layer)
		if err != nil {
			return err
		}
		b.layer = l.(*layers.Conv2D)
	}
	return nil
}
