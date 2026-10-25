package threemm

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
	NI               int
	NJ               int
	NK               int
	NL               int
	NM               int
	A                []float32
	B                []float32
	C                []float32
	D                []float32
	GA               driver.Ptr
	GB               driver.Ptr
	GC               driver.Ptr
	GD               driver.Ptr
	GE               driver.Ptr
	GF               driver.Ptr
	GG               driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NI               int
	NJ               int
	NK               int
	NL               int
	NM               int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		NI:               b.NI,
		NJ:               b.NJ,
		NK:               b.NK,
		NL:               b.NL,
		NM:               b.NM,
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
		NI:               b.NI,
		NJ:               b.NJ,
		NK:               b.NK,
		NL:               b.NL,
		NM:               b.NM,
		A:                b.a,
		B:                b.b,
		C:                b.c,
		D:                b.d,
		GA:               b.gA,
		GB:               b.gB,
		GC:               b.gC,
		GD:               b.gD,
		GE:               b.gE,
		GF:               b.gF,
		GG:               b.gG,
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
	b.NI = s.NI
	b.NJ = s.NJ
	b.NK = s.NK
	b.NL = s.NL
	b.NM = s.NM
	b.a = s.A
	b.b = s.B
	b.c = s.C
	b.d = s.D
	b.gA = s.GA
	b.gB = s.GB
	b.gC = s.GC
	b.gD = s.GD
	b.gE = s.GE
	b.gF = s.GF
	b.gG = s.GG
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
