// Package xor trains a small fully connected network on XOR.
package xor

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputraining"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training/optimization"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type Benchmark struct {
	benchmarks.Progress
	driver                             *driver.Runtime
	ctx                                driver.Context
	gpus                               []int
	Arch                               arch.Type
	Epoch, MaxBatchPerEpoch, BatchSize int
	EnableVerification, EnableTesting  bool
	trainer                            gputraining.DataParallelismMultiGPUTrainer
}

func NewBenchmark(r *driver.Runtime) *Benchmark {
	return &Benchmark{driver: r, ctx: r.Init(), Epoch: 50, MaxBatchPerEpoch: 1, BatchSize: 4}
}
func (b *Benchmark) SelectGPU(ids []int) {
	if len(ids) != 1 {
		panic("XOR requires one GPU")
	}
	b.gpus = append([]int(nil), ids...)
}
func (b *Benchmark) SetUnifiedMemory() { panic("unified memory is not supported by dnn workloads") }
func (b *Benchmark) SetVerification(enabled bool) {
	b.Progress.SetVerification(enabled)
	b.EnableVerification = enabled
}
func (b *Benchmark) init() {
	b.driver.SelectGPU(b.ctx, b.gpus[0])
	o := gputensor.NewGPUOperator(b.driver, b.ctx)
	o.Arch = b.Arch
	o.SetVerification(b.EnableVerification)
	n := training.Network{Layers: []layers.Layer{
		layers.NewFullyConnectedLayer(0, o, 2, 4),
		layers.NewReluLayer(o),
		layers.NewFullyConnectedLayer(2, o, 4, 2),
	}}
	b.trainer = gputraining.DataParallelismMultiGPUTrainer{
		Driver:           b.driver,
		TensorOperators:  []*gputensor.GPUOperator{o},
		Networks:         []training.Network{n},
		DataSource:       []training.DataSource{NewDataSource(o)},
		LossFunc:         []training.LossFunction{training.NewSoftmaxCrossEntropy(o)},
		OptimizationAlg:  []optimization.Alg{optimization.NewAdam(o, 0.03)},
		GPUs:             b.gpus,
		Contexts:         []driver.Context{b.ctx},
		Epoch:            b.Epoch,
		MaxBatchPerEpoch: b.MaxBatchPerEpoch,
		BatchSize:        b.BatchSize,
		ShowBatchInfo:    true,
	}
	if b.EnableTesting {
		b.trainer.Tester = []*training.Tester{{DataSource: NewDataSource(o), Network: n, BatchSize: 4}}
	}
}
func (b *Benchmark) Step() error {
	if b.Ended {
		return nil
	}
	if b.Phase == 0 {
		b.init()
		b.Phase = 1
	}
	if err := b.trainer.Step(); err != nil {
		return err
	}
	b.Ended = b.trainer.IsEnded()
	return nil
}
