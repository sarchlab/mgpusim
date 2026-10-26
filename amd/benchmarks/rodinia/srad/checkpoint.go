package srad

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress
	Readbacks        []driver.HostBuffer
	Context          driver.Context
	Queue            driver.CommandQueue
	Gpus             []int
	Arch             arch.Type
	ImageSize        int       // image is ImageSize x ImageSize
	NumIterations    int       // SRAD iterations (srad1+srad2 pairs) per run
	JInit            []float32 // initial image (reproduced in Verify)
	GJ               driver.Ptr
	GDN              driver.Ptr
	GDS              driver.Ptr
	GDW              driver.Ptr
	GDE              driver.Ptr
	GC               driver.Ptr
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
