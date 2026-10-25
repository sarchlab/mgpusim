package xor

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputraining"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"io"
)

type checkpointConfig struct {
	GPUs                               []int
	Arch                               arch.Type
	Epoch, BatchSize, MaxBatchPerEpoch int
	Verification, Testing              bool
}
type checkpointState struct {
	Progress benchmarks.Progress
	Context  driver.Context
	Trainer  gputraining.State
}

func (b *Benchmark) configuration() checkpointConfig {
	return checkpointConfig{
		b.gpus,
		b.Arch,
		b.Epoch,
		b.BatchSize,
		b.MaxBatchPerEpoch,
		b.EnableVerification,
		b.EnableTesting,
	}
}
func (b *Benchmark) SaveCheckpoint(w io.Writer) error {
	s, err := b.trainer.Snapshot()
	if err != nil {
		return err
	}
	return modeling.WriteCheckpoint(w, b.configuration(), checkpointState{b.Progress, b.ctx, s}, nil)
}
func (b *Benchmark) LoadCheckpoint(r io.Reader) error {
	var s checkpointState
	if err := modeling.ReadCheckpoint(r, b.configuration(), &s, nil); err != nil {
		return err
	}
	t, err := gputraining.Restore(b.driver, s.Trainer)
	if err != nil {
		return err
	}
	b.Progress = s.Progress
	b.ctx = s.Context
	b.trainer = t
	return nil
}
