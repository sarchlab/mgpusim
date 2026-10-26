package vgg16

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputraining"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type checkpointConfig struct {
	GPUs                               []int
	Arch                               arch.Type
	Epoch, BatchSize, MaxBatchPerEpoch int
	Verification, Testing              bool
}
type State struct {
	benchmarks.Progress
	Context                            driver.Context
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
	t, err := gputraining.Bind(b.driver, &b.Trainer)
	if err != nil {
		return err
	}
	b.trainer = t
	b.to = t.TensorOperators
	b.contexts = t.Contexts
	b.networks = t.Networks
	return nil
}
