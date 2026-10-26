package cachelatency

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress
	Readbacks []driver.CommandID
	Context   driver.Context
	Queue     driver.CommandQueue
	Gpus      []int
	Arch      arch.Type
	// ArrayBytes is the size of the pointer-chasing array in bytes.
	ArrayBytes int
	// CachelineBytes is the chase granularity: one chase node per cacheline, so
	// consecutive accesses land on distinct lines (cache footprint == ArrayBytes).
	// Matches the real benchmark (gfx942 line = 64 B).
	CachelineBytes int
	// MeasureLaps sets the timed access count when NumAccesses is auto (<=0):
	// clamp(MeasureLaps * lines, minTimedAccesses, maxTimedAccesses), matching the
	// real benchmark so total kernel time is comparable.
	MeasureLaps int
	// NumAccesses is the number of dependent loads the single thread performs.
	// If <=0 it is derived from MeasureLaps (see initMem); an explicit value wins.
	NumAccesses int
	// Seed controls the deterministic chain permutation.
	Seed             uint32
	N                int // total array elements (ArrayBytes/4)
	M                int // number of cacheline chase nodes (n/stride)
	Stride           int // element stride between nodes (CachelineBytes/4)
	StartIdx         uint32
	Chain            []uint32
	GArr             driver.Ptr
	GResult          driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	ArrayBytes       int
	CachelineBytes   int
	MeasureLaps      int
	NumAccesses      int
	Seed             uint32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		ArrayBytes:       b.ArrayBytes,
		CachelineBytes:   b.CachelineBytes,
		MeasureLaps:      b.MeasureLaps,
		NumAccesses:      b.NumAccesses,
		Seed:             b.Seed,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
