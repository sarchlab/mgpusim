package threemm

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress
	Readbacks []gpuruntime.HostBuffer
	Context   gpuruntime.Context
	Queue     gpuruntime.CommandQueue
	Gpus      []int
	Arch      arch.Type
	// Matrix dimensions. All default to N (set via NewBenchmark caller).
	NI               int
	NJ               int
	NK               int
	NL               int
	NM               int
	A                []float32 // NI×NK
	B                []float32 // NK×NJ
	C                []float32 // NJ×NM
	D                []float32 // NM×NL
	GA               gpuruntime.Ptr
	GB               gpuruntime.Ptr
	GC               gpuruntime.Ptr
	GD               gpuruntime.Ptr
	GE               gpuruntime.Ptr
	GF               gpuruntime.Ptr
	GG               gpuruntime.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NI               int
	NJ               int
	NK               int
	NL               int
	NM               int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		NI:               b.NI,
		NJ:               b.NJ,
		NK:               b.NK,
		NL:               b.NL,
		NM:               b.NM,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
