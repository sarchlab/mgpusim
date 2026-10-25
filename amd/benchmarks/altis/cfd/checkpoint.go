package cfd

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"io"
)

type checkpointState struct {
	InitialConfig    checkpointConfig
	ConfigCaptured   bool
	Progress         benchmarks.Progress
	Readbacks        []driver.CommandID
	Context          driver.Context
	Queue            driver.CommandQueue
	Gpus             []int
	Arch             arch.Type
	N                int
	HRho             []float32
	HMx              []float32
	HMy              []float32
	HMz              []float32
	HEnergy          []float32
	HNeighbors       []int32
	HNormals         []float32
	HAreas           []float32
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
		Gpus:             b.gpus,
		Arch:             b.Arch,
		N:                b.N,
		UseUnifiedMemory: b.useUnifiedMemory,
	}
}
func (b *Benchmark) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, b.configuration(), checkpointState{
		InitialConfig:    b.initialConfig,
		ConfigCaptured:   b.configCaptured,
		Progress:         b.Progress,
		Readbacks:        b.readbacks,
		Context:          b.context,
		Queue:            b.queue,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		N:                b.N,
		HRho:             b.hRho,
		HMx:              b.hMx,
		HMy:              b.hMy,
		HMz:              b.hMz,
		HEnergy:          b.hEnergy,
		HNeighbors:       b.hNeighbors,
		HNormals:         b.hNormals,
		HAreas:           b.hAreas,
		GRho:             b.gRho,
		GMx:              b.gMx,
		GMy:              b.gMy,
		GMz:              b.gMz,
		GEnergy:          b.gEnergy,
		GNeighbors:       b.gNeighbors,
		GNormals:         b.gNormals,
		GAreas:           b.gAreas,
		GFluxRho:         b.gFluxRho,
		GFluxMx:          b.gFluxMx,
		GFluxMy:          b.gFluxMy,
		GFluxMz:          b.gFluxMz,
		GFluxEnergy:      b.gFluxEnergy,
		UseUnifiedMemory: b.useUnifiedMemory,
	}, nil)
}
func (b *Benchmark) LoadCheckpoint(r io.Reader) error {
	var s checkpointState
	if err := modeling.ReadCheckpoint(r, b.configuration(), &s, nil); err != nil {
		return err
	}
	b.initialConfig = s.InitialConfig
	b.configCaptured = s.ConfigCaptured
	b.Progress = s.Progress
	b.readbacks = s.Readbacks
	b.context = s.Context
	b.queue = s.Queue
	b.gpus = s.Gpus
	b.Arch = s.Arch
	b.N = s.N
	b.hRho = s.HRho
	b.hMx = s.HMx
	b.hMy = s.HMy
	b.hMz = s.HMz
	b.hEnergy = s.HEnergy
	b.hNeighbors = s.HNeighbors
	b.hNormals = s.HNormals
	b.hAreas = s.HAreas
	b.gRho = s.GRho
	b.gMx = s.GMx
	b.gMy = s.GMy
	b.gMz = s.GMz
	b.gEnergy = s.GEnergy
	b.gNeighbors = s.GNeighbors
	b.gNormals = s.GNormals
	b.gAreas = s.GAreas
	b.gFluxRho = s.GFluxRho
	b.gFluxMx = s.GFluxMx
	b.gFluxMy = s.GFluxMy
	b.gFluxMz = s.GFluxMz
	b.gFluxEnergy = s.GFluxEnergy
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadProgram()
	return nil
}

func (b *Benchmark) configuration() checkpointConfig {
	if b.configCaptured {
		return b.initialConfig
	}
	return b.checkpointConfig()
}
