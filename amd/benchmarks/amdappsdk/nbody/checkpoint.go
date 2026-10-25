package nbody

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
	Readbacks        []benchmarks.Readback
	Queue            driver.CommandQueue
	Context          driver.Context
	Gpus             []int
	Queues           []driver.CommandQueue
	UseUnifiedMemory bool
	Arch             arch.Type
	NumParticles     int32
	DelT             float32
	EspSqr           float32
	InitPos          []float32
	InitVel          []float32
	Pos              []float32
	Vel              []float32
	RefPos           []float32
	RefVel           []float32
	GroupSize        int32
	NumIterations    int32
	Exchange         bool
	NumBodies        int32
	CurrPos          driver.Ptr
	CurrVel          driver.Ptr
	NewPos           driver.Ptr
	NewVel           driver.Ptr
	DPos             driver.Ptr
	DVel             driver.Ptr
	DNewPos          driver.Ptr
	DNewVel          driver.Ptr
	LocalPosBuf      driver.Ptr
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	UseUnifiedMemory bool
	Arch             arch.Type
	NumParticles     int32
	NumIterations    int32
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		UseUnifiedMemory: b.useUnifiedMemory,
		Arch:             b.Arch,
		NumParticles:     b.NumParticles,
		NumIterations:    b.NumIterations,
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
		Readbacks:        b.readbacks,
		Queue:            b.queue,
		Context:          b.context,
		Gpus:             b.gpus,
		Queues:           b.queues,
		UseUnifiedMemory: b.useUnifiedMemory,
		Arch:             b.Arch,
		NumParticles:     b.NumParticles,
		DelT:             b.delT,
		EspSqr:           b.espSqr,
		InitPos:          b.initPos,
		InitVel:          b.initVel,
		Pos:              b.pos,
		Vel:              b.vel,
		RefPos:           b.refPos,
		RefVel:           b.refVel,
		GroupSize:        b.groupSize,
		NumIterations:    b.NumIterations,
		Exchange:         b.exchange,
		NumBodies:        b.numBodies,
		CurrPos:          b.currPos,
		CurrVel:          b.currVel,
		NewPos:           b.newPos,
		NewVel:           b.newVel,
		DPos:             b.dPos,
		DVel:             b.dVel,
		DNewPos:          b.dNewPos,
		DNewVel:          b.dNewVel,
		LocalPosBuf:      b.localPosBuf,
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
	b.readbacks = s.Readbacks
	b.queue = s.Queue
	b.context = s.Context
	b.gpus = s.Gpus
	b.queues = s.Queues
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.Arch = s.Arch
	b.NumParticles = s.NumParticles
	b.delT = s.DelT
	b.espSqr = s.EspSqr
	b.initPos = s.InitPos
	b.initVel = s.InitVel
	b.pos = s.Pos
	b.vel = s.Vel
	b.refPos = s.RefPos
	b.refVel = s.RefVel
	b.groupSize = s.GroupSize
	b.NumIterations = s.NumIterations
	b.exchange = s.Exchange
	b.numBodies = s.NumBodies
	b.currPos = s.CurrPos
	b.currVel = s.CurrVel
	b.newPos = s.NewPos
	b.newVel = s.NewVel
	b.dPos = s.DPos
	b.dVel = s.DVel
	b.dNewPos = s.DNewPos
	b.dNewVel = s.DNewVel
	b.localPosBuf = s.LocalPosBuf
	b.loadProgram()
	return nil
}
