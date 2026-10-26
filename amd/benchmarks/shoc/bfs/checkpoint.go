package bfs

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress

	Depth            int
	FlagResult       driver.HostBuffer
	OutputResult     driver.HostBuffer
	Queue            driver.CommandQueue
	Context          driver.Context
	Gpus             []int
	Queues           []driver.CommandQueue
	Arch             arch.Type
	Path             string
	NumNode          int
	Degree           int
	MaxDepth         int
	SourceNode       int
	HEdgeOffsets     []int32
	HEdgeList        []int32
	HFrontier        []uint32
	HCost            []uint32
	CpuCost          []uint32
	DFrontier        driver.Ptr
	DEdgeArray       driver.Ptr
	DEdgeArrayAux    driver.Ptr
	DFlag            driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	Path             string
	NumNode          int
	Degree           int
	MaxDepth         int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		Path:             b.Path,
		NumNode:          b.NumNode,
		Degree:           b.Degree,
		MaxDepth:         b.MaxDepth,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
