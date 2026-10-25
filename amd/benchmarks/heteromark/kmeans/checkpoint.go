package kmeans

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"io"
)

type checkpointState struct {
	Progress         benchmarks.Progress
	InitialConfig    checkpointConfig
	ConfigCaptured   bool
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
		Gpus:             b.gpus,
		Arch:             b.Arch,
		NumClusters:      b.NumClusters,
		NumPoints:        b.NumPoints,
		NumFeatures:      b.NumFeatures,
		MaxIter:          b.MaxIter,
		UseUnifiedMemory: b.useUnifiedMemory,
	}
}
func (b *Benchmark) configuration() checkpointConfig {
	if b.configCaptured {
		return b.initialConfig
	}
	return b.checkpointConfig()
}
func (b *Benchmark) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, b.configuration(), checkpointState{
		Progress:         b.Progress,
		InitialConfig:    b.initialConfig,
		ConfigCaptured:   b.configCaptured,
		Iteration:        b.iteration,
		Readback:         b.readback,
		SwapReadback:     b.swapReadback,
		Context:          b.context,
		Gpus:             b.gpus,
		Queues:           b.queues,
		Arch:             b.Arch,
		NumClusters:      b.NumClusters,
		NumPoints:        b.NumPoints,
		NumFeatures:      b.NumFeatures,
		MaxIter:          b.MaxIter,
		HFeatures:        b.hFeatures,
		DFeatures:        b.dFeatures,
		DFeaturesSwap:    b.dFeaturesSwap,
		HMembership:      b.hMembership,
		DMembership:      b.dMembership,
		HClusters:        b.hClusters,
		DClusters:        b.dClusters,
		GpuRMSE:          b.gpuRMSE,
		UseUnifiedMemory: b.useUnifiedMemory,
	}, nil)
}
func (b *Benchmark) LoadCheckpoint(r io.Reader) error {
	var s checkpointState
	if err := modeling.ReadCheckpoint(r, b.configuration(), &s, nil); err != nil {
		return err
	}
	b.Progress = s.Progress
	b.initialConfig = s.InitialConfig
	b.configCaptured = s.ConfigCaptured
	b.iteration = s.Iteration
	b.readback = s.Readback
	b.swapReadback = s.SwapReadback
	b.context = s.Context
	b.gpus = s.Gpus
	b.queues = s.Queues
	b.Arch = s.Arch
	b.NumClusters = s.NumClusters
	b.NumPoints = s.NumPoints
	b.NumFeatures = s.NumFeatures
	b.MaxIter = s.MaxIter
	b.hFeatures = s.HFeatures
	b.dFeatures = s.DFeatures
	b.dFeaturesSwap = s.DFeaturesSwap
	b.hMembership = s.HMembership
	b.dMembership = s.DMembership
	b.hClusters = s.HClusters
	b.dClusters = s.DClusters
	b.gpuRMSE = s.GpuRMSE
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadKernels()
	return nil
}
