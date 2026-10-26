package im2col

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	Operator *gputensor.OperatorState
	InputID  tensor.ID

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
		Gpus:             b.Gpus,
		UseUnifiedMemory: b.UseUnifiedMemory,
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
func (b *Benchmark) bind() error {
	b.operator = nil
	if b.Operator != nil {
		b.operator = gputensor.BindOperator(b.driver, b.Operator)
	}
	return nil
}
