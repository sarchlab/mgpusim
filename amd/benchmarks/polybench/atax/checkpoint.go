package atax

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
	Arch             arch.Type
	NX               int
	NY               int
	A                []float32
	X                []float32
	Y                []float32
	YOutput          []float32
	Tmp              []float32
	DA               driver.Ptr
	DX               driver.Ptr
	DY               driver.Ptr
	DTmp             driver.Ptr
	CpuY             []float32
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NX               int
	NY               int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		NX:               b.NX,
		NY:               b.NY,
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
		Readbacks:        b.readbacks,
		Queue:            b.queue,
		Context:          b.context,
		Gpus:             b.gpus,
		Queues:           b.queues,
		Arch:             b.Arch,
		NX:               b.NX,
		NY:               b.NY,
		A:                b.a,
		X:                b.x,
		Y:                b.y,
		YOutput:          b.yOutput,
		Tmp:              b.tmp,
		DA:               b.dA,
		DX:               b.dX,
		DY:               b.dY,
		DTmp:             b.dTmp,
		CpuY:             b.cpuY,
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
	b.readbacks = s.Readbacks
	b.queue = s.Queue
	b.context = s.Context
	b.gpus = s.Gpus
	b.queues = s.Queues
	b.Arch = s.Arch
	b.NX = s.NX
	b.NY = s.NY
	b.a = s.A
	b.x = s.X
	b.y = s.Y
	b.yOutput = s.YOutput
	b.tmp = s.Tmp
	b.dA = s.DA
	b.dX = s.DX
	b.dY = s.DY
	b.dTmp = s.DTmp
	b.cpuY = s.CpuY
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadProgram()
	return nil
}
