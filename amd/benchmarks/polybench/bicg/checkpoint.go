package bicg

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
	R                []float32
	S                []float32
	P                []float32
	Q                []float32
	SOutput          []float32
	QOutput          []float32
	CpuS             []float32
	CpuQ             []float32
	DA               driver.Ptr
	DR               driver.Ptr
	DS               driver.Ptr
	DP               driver.Ptr
	DQ               driver.Ptr
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
		R:                b.r,
		S:                b.s,
		P:                b.p,
		Q:                b.q,
		SOutput:          b.sOutput,
		QOutput:          b.qOutput,
		CpuS:             b.cpuS,
		CpuQ:             b.cpuQ,
		DA:               b.dA,
		DR:               b.dR,
		DS:               b.dS,
		DP:               b.dP,
		DQ:               b.dQ,
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
	b.r = s.R
	b.s = s.S
	b.p = s.P
	b.q = s.Q
	b.sOutput = s.SOutput
	b.qOutput = s.QOutput
	b.cpuS = s.CpuS
	b.cpuQ = s.CpuQ
	b.dA = s.DA
	b.dR = s.DR
	b.dS = s.DS
	b.dP = s.DP
	b.dQ = s.DQ
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadProgram()
	return nil
}
