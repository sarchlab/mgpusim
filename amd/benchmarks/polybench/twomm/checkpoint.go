package twomm

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress
	Readbacks        []gpuruntime.HostBuffer
	Context          gpuruntime.Context
	Queue            gpuruntime.CommandQueue
	Gpus             []int
	Arch             arch.Type
	N                int
	A                []float32
	B                []float32
	C                []float32
	DInit            []float32
	EInit            []float32
	GA               gpuruntime.Ptr
	GB               gpuruntime.Ptr
	GC               gpuruntime.Ptr
	GD               gpuruntime.Ptr
	GE               gpuruntime.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	N                int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		N:                b.N,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
