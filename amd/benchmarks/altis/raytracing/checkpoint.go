package raytracing

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
	Width            int
	Height           int
	Spheres          int
	SphereData       []float32 // flat [numSpheres*7] sphere data
	GImage           gpuruntime.Ptr
	GSpheres         gpuruntime.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	Width            int
	Height           int
	Spheres          int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		Width:            b.Width,
		Height:           b.Height,
		Spheres:          b.Spheres,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
