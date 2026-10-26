package conv2d

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	Operator                  *gputensor.OperatorState
	Layer                     layers.State
	ForwardInID, BackwardInID tensor.ID

	benchmarks.Progress
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
		Gpus:             b.Gpus,
		UseUnifiedMemory: b.UseUnifiedMemory,
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
func (b *Benchmark) bind() error {
	b.operator = nil
	if b.Operator != nil {
		b.operator = gputensor.BindOperator(b.driver, b.Operator)
	}
	b.layer = nil
	if b.Operator != nil {
		l, err := layers.Bind(b.operator, b.Layer)
		if err != nil {
			return err
		}
		b.layer = l.(*layers.Conv2D)
	}
	return nil
}
