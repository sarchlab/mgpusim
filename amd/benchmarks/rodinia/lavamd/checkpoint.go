package lavamd

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
	NumBoxes         int
	ParticlesPerBox  int
	TotalBoxes       int
	TotalParticles   int
	HPosX            []float32
	HPosY            []float32
	HPosZ            []float32
	HNeighborList    []int32
	HNeighborCount   []int32
	GPosX            driver.Ptr
	GPosY            driver.Ptr
	GPosZ            driver.Ptr
	GForceX          driver.Ptr
	GForceY          driver.Ptr
	GForceZ          driver.Ptr
	GEnergy          driver.Ptr
	GNeighborList    driver.Ptr
	GNeighborCount   driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NumBoxes         int
	ParticlesPerBox  int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		NumBoxes:         b.NumBoxes,
		ParticlesPerBox:  b.ParticlesPerBox,
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
		NumBoxes:         b.NumBoxes,
		ParticlesPerBox:  b.ParticlesPerBox,
		TotalBoxes:       b.totalBoxes,
		TotalParticles:   b.totalParticles,
		HPosX:            b.hPosX,
		HPosY:            b.hPosY,
		HPosZ:            b.hPosZ,
		HNeighborList:    b.hNeighborList,
		HNeighborCount:   b.hNeighborCount,
		GPosX:            b.gPosX,
		GPosY:            b.gPosY,
		GPosZ:            b.gPosZ,
		GForceX:          b.gForceX,
		GForceY:          b.gForceY,
		GForceZ:          b.gForceZ,
		GEnergy:          b.gEnergy,
		GNeighborList:    b.gNeighborList,
		GNeighborCount:   b.gNeighborCount,
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
	b.NumBoxes = s.NumBoxes
	b.ParticlesPerBox = s.ParticlesPerBox
	b.totalBoxes = s.TotalBoxes
	b.totalParticles = s.TotalParticles
	b.hPosX = s.HPosX
	b.hPosY = s.HPosY
	b.hPosZ = s.HPosZ
	b.hNeighborList = s.HNeighborList
	b.hNeighborCount = s.HNeighborCount
	b.gPosX = s.GPosX
	b.gPosY = s.GPosY
	b.gPosZ = s.GPosZ
	b.gForceX = s.GForceX
	b.gForceY = s.GForceY
	b.gForceZ = s.GForceZ
	b.gEnergy = s.GEnergy
	b.gNeighborList = s.GNeighborList
	b.gNeighborCount = s.GNeighborCount
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
