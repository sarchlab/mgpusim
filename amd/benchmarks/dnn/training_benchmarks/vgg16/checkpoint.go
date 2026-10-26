package vgg16

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputraining"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type checkpointConfig struct {
	GPUs                               []int
	Arch                               arch.Type
	Epoch, BatchSize, MaxBatchPerEpoch int
	Verification, Testing              bool
}
type State struct {
	benchmarks.Progress
	Context                            gpuruntime.Context
	GPUs                               []int
	Arch                               arch.Type
	Epoch, BatchSize, MaxBatchPerEpoch int
	EnableVerification, EnableTesting  bool
	Trainer                            gputraining.State
}

func (b *Benchmark) configuration() checkpointConfig {
	return checkpointConfig{
		b.GPUs,
		b.Arch,
		b.Epoch,
		b.BatchSize,
		b.MaxBatchPerEpoch,
		b.EnableVerification,
		b.EnableTesting,
	}
}

func (b *Benchmark) bind() error {
	t, err := gputraining.Bind(b.rt, &b.Trainer)
	if err != nil {
		return err
	}
	b.trainer = t
	b.to = t.TensorOperators
	b.contexts = t.Contexts
	b.networks = t.Networks
	return nil
}
