// Package xor trains a small fully connected network on XOR.
package xor

import (
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputraining"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training/optimization"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type Benchmark struct {
	*State
	benchmarks.Saved[checkpointConfig, State]
	driver  *driver.Runtime
	trainer gputraining.DataParallelismMultiGPUTrainer
}

func NewBenchmark(r *driver.Runtime) *Benchmark {
	b := &Benchmark{State: &State{Context: r.Init(), Epoch: 50, MaxBatchPerEpoch: 1, BatchSize: 4}, driver: r}
	b.Saved = benchmarks.NewSaved(b.State, b.configuration, b.bind)
	return b
}
func (b *Benchmark) SelectGPU(ids []int) {
	if len(ids) != 1 {
		panic("XOR requires one GPU")
	}
	b.GPUs = append([]int(nil), ids...)
}
func (b *Benchmark) SetUnifiedMemory() { panic("unified memory is not supported by dnn workloads") }
func (b *Benchmark) SetVerification(enabled bool) {
	b.Progress.SetVerification(enabled)
	b.EnableVerification = enabled
}
func (b *Benchmark) init() error {
	b.driver.SelectGPU(b.Context, b.GPUs[0])
	o := gputensor.NewGPUOperator(b.driver, b.Context)
	o.Arch = b.Arch
	o.SetVerification(b.EnableVerification)
	n := training.Network{Layers: []layers.Layer{
		layers.NewFullyConnectedLayer(0, o, 2, 4),
		layers.NewReluLayer(o),
		layers.NewFullyConnectedLayer(2, o, 4, 2),
	}}
	b.Trainer = gputraining.State{
		Epoch:            b.Epoch,
		MaxBatchPerEpoch: b.MaxBatchPerEpoch,
		BatchSize:        b.BatchSize,
		ShowBatchInfo:    true,
		GPUs:             b.GPUs,
		Contexts: []driver.Context{
			b.Context,
		},
	}
	b.trainer = gputraining.DataParallelismMultiGPUTrainer{State: &b.Trainer,
		Driver:          b.driver,
		TensorOperators: []*gputensor.GPUOperator{o},
		Networks:        []training.Network{n},
		DataSource:      []training.DataSource{NewDataSource(o)},
		LossFunc:        []training.LossFunction{training.NewSoftmaxCrossEntropy(o)},
		OptimizationAlg: []optimization.Alg{optimization.NewAdam(o, 0.03)},
	}
	if b.EnableTesting {
		b.trainer.Tester = []*training.Tester{{DataSource: NewDataSource(o), Network: n, BatchSize: 4}}
	}
	return b.trainer.Connect()
}
func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.driver); err != nil {
		return err
	}
	if b.Ended {
		return nil
	}
	if b.Phase == 0 {
		if err := b.init(); err != nil {
			return err
		}
		b.Phase = 1
	}
	if err := b.trainer.Step(); err != nil {
		return err
	}
	b.Ended = b.trainer.IsEnded()
	return nil
}
