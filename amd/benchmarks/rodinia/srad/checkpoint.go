package srad

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
	ImageSize        int       // image is ImageSize x ImageSize
	NumIterations    int       // SRAD iterations (srad1+srad2 pairs) per run
	JInit            []float32 // initial image (reproduced in Verify)
	GJ               gpuruntime.Ptr
	GDN              gpuruntime.Ptr
	GDS              gpuruntime.Ptr
	GDW              gpuruntime.Ptr
	GDE              gpuruntime.Ptr
	GC               gpuruntime.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	ImageSize        int
	NumIterations    int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		ImageSize:        b.ImageSize,
		NumIterations:    b.NumIterations,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
