package backprop

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
	InputN           int
	HiddenN          int
	OutputN          int
	HInput           []float32
	HW1              []float32
	HW2              []float32
	HB1              []float32
	HB2              []float32
	HTarget          []float32
	GInput           driver.Ptr
	GW1              driver.Ptr
	GB1              driver.Ptr
	GHidden          driver.Ptr
	GW2              driver.Ptr
	GB2              driver.Ptr
	GOutput          driver.Ptr
	GTarget          driver.Ptr
	GDeltaOut        driver.Ptr
	GDeltaHid        driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	InputN           int
	HiddenN          int
	OutputN          int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		InputN:           b.InputN,
		HiddenN:          b.HiddenN,
		OutputN:          b.OutputN,
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
		InputN:           b.InputN,
		HiddenN:          b.HiddenN,
		OutputN:          b.OutputN,
		HInput:           b.hInput,
		HW1:              b.hW1,
		HW2:              b.hW2,
		HB1:              b.hB1,
		HB2:              b.hB2,
		HTarget:          b.hTarget,
		GInput:           b.gInput,
		GW1:              b.gW1,
		GB1:              b.gB1,
		GHidden:          b.gHidden,
		GW2:              b.gW2,
		GB2:              b.gB2,
		GOutput:          b.gOutput,
		GTarget:          b.gTarget,
		GDeltaOut:        b.gDeltaOut,
		GDeltaHid:        b.gDeltaHid,
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
	b.InputN = s.InputN
	b.HiddenN = s.HiddenN
	b.OutputN = s.OutputN
	b.hInput = s.HInput
	b.hW1 = s.HW1
	b.hW2 = s.HW2
	b.hB1 = s.HB1
	b.hB2 = s.HB2
	b.hTarget = s.HTarget
	b.gInput = s.GInput
	b.gW1 = s.GW1
	b.gB1 = s.GB1
	b.gHidden = s.GHidden
	b.gW2 = s.GW2
	b.gB2 = s.GB2
	b.gOutput = s.GOutput
	b.gTarget = s.GTarget
	b.gDeltaOut = s.GDeltaOut
	b.gDeltaHid = s.GDeltaHid
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
