package cfd

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress
	Readbacks []driver.HostBuffer
	Context   driver.Context
	Queue     driver.CommandQueue
	Gpus      []int
	Arch      arch.Type
	N         int
	// Host data (deterministic, reproducible in Verify).
	HRho       []float32
	HMx        []float32
	HMy        []float32
	HMz        []float32
	HEnergy    []float32
	HNeighbors []int32
	HNormals   []float32
	HAreas     []float32
	// Device buffers.
	GRho             driver.Ptr
	GMx              driver.Ptr
	GMy              driver.Ptr
	GMz              driver.Ptr
	GEnergy          driver.Ptr
	GNeighbors       driver.Ptr
	GNormals         driver.Ptr
	GAreas           driver.Ptr
	GFluxRho         driver.Ptr
	GFluxMx          driver.Ptr
	GFluxMy          driver.Ptr
	GFluxMz          driver.Ptr
	GFluxEnergy      driver.Ptr
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
