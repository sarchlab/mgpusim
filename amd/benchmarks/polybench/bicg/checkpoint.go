package bicg

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress

	Readbacks        []benchmarks.Readback
	Queue            gpuruntime.CommandQueue
	Context          gpuruntime.Context
	Gpus             []int
	Arch             arch.Type
	NX               int
	NY               int
	A                []float32
	R                []float32
	S                []float32
	P                []float32
	Q                []float32
	SOutput          []float32
	QOutput          []float32
	CpuS             []float32
	CpuQ             []float32
	DA               gpuruntime.Ptr
	DR               gpuruntime.Ptr
	DS               gpuruntime.Ptr
	DP               gpuruntime.Ptr
	DQ               gpuruntime.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NX               int
	NY               int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		NX:               b.NX,
		NY:               b.NY,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
