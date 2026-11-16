package minerva

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputraining"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress
	Context                            gpuruntime.Context
	GPUs                               []int
	Arch                               arch.Type
	Epoch, BatchSize, MaxBatchPerEpoch int
	EnableVerification, EnableTesting  bool
	Trainer                            gputraining.State
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
