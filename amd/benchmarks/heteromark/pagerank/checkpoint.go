package pagerank

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/matrix/csr"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress

	Readbacks        []benchmarks.Readback
	Queue            driver.CommandQueue
	Context          driver.Context
	Gpus             []int
	Arch             arch.Type
	NumNodes         uint32
	NumConnections   uint32
	MaxIterations    uint32
	HMatrix          csr.Matrix
	HPageRank        []float32
	VerPageRank      []float32
	VerPageRankTemp  []float32
	DPageRank        driver.Ptr
	DPageRankTemp    driver.Ptr
	DRowOffsets      driver.Ptr
	DColumnNumbers   driver.Ptr
	DValues          driver.Ptr
	DLocalValues     driver.LocalPtr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NumNodes         uint32
	NumConnections   uint32
	MaxIterations    uint32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		NumNodes:         b.NumNodes,
		NumConnections:   b.NumConnections,
		MaxIterations:    b.MaxIterations,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
