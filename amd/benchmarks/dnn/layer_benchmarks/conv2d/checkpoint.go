package conv2d

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	Operator                  *gputensor.OperatorState
	Layer                     layers.State
	ForwardInID, BackwardInID tensor.ID

	benchmarks.Progress
	Context          gpuruntime.Context
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

func (b *Benchmark) bind() error {
	b.operator = nil
	if b.Operator != nil {
		b.operator = gputensor.BindOperator(b.rt, b.Operator)
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
