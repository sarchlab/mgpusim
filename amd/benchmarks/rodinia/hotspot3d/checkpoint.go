package hotspot3d

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
	GridSize         int
	NumIterations    int
	AmbTemp          float32
	StepDivCap       float32
	Rx1              float32
	Ry1              float32
	Rz1              float32
	Ra1              float32
	TempInit         []float32
	Power            []float32
	GTemp            [2]driver.Ptr
	GPower           driver.Ptr
	FinalBuf         int
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	GridSize         int
	NumIterations    int
	AmbTemp          float32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		GridSize:         b.GridSize,
		NumIterations:    b.NumIterations,
		AmbTemp:          b.AmbTemp,
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
		GridSize:         b.GridSize,
		NumIterations:    b.NumIterations,
		AmbTemp:          b.AmbTemp,
		StepDivCap:       b.stepDivCap,
		Rx1:              b.rx1,
		Ry1:              b.ry1,
		Rz1:              b.rz1,
		Ra1:              b.ra1,
		TempInit:         b.tempInit,
		Power:            b.power,
		GTemp:            b.gTemp,
		GPower:           b.gPower,
		FinalBuf:         b.finalBuf,
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
	b.GridSize = s.GridSize
	b.NumIterations = s.NumIterations
	b.AmbTemp = s.AmbTemp
	b.stepDivCap = s.StepDivCap
	b.rx1 = s.Rx1
	b.ry1 = s.Ry1
	b.rz1 = s.Rz1
	b.ra1 = s.Ra1
	b.tempInit = s.TempInit
	b.power = s.Power
	b.gTemp = s.GTemp
	b.gPower = s.GPower
	b.finalBuf = s.FinalBuf
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
