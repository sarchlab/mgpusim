package kmeans

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress

	Iteration        int
	Readback         driver.CommandID
	SwapReadback     driver.CommandID
	Context          driver.Context
	Gpus             []int
	Queues           []driver.CommandQueue
	Arch             arch.Type
	NumClusters      int
	NumPoints        int
	NumFeatures      int
	MaxIter          int
	HFeatures        []float32
	DFeatures        driver.Ptr
	DFeaturesSwap    driver.Ptr
	HMembership      []int32
	DMembership      driver.Ptr
	HClusters        []float32
	DClusters        []driver.Ptr
	GpuRMSE          float64
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NumClusters      int
	NumPoints        int
	NumFeatures      int
	MaxIter          int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		NumClusters:      b.NumClusters,
		NumPoints:        b.NumPoints,
		NumFeatures:      b.NumFeatures,
		MaxIter:          b.MaxIter,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadKernels()
	return nil
}
