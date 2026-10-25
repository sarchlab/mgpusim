package matrixmultiplication

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
	Queue            driver.CommandQueue
	BlockABuf        driver.Ptr
	GA               driver.Ptr
	GB               driver.Ptr
	GC               driver.Ptr
	Readback         driver.CommandID
	Context          driver.Context
	Gpus             []int
	Arch             arch.Type
	X                uint32
	Y                uint32
	Z                uint32
	MatrixA          Matrix
	MatrixB          Matrix
	MatrixC          Matrix
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	X                uint32
	Y                uint32
	Z                uint32
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		X:                b.X,
		Y:                b.Y,
		Z:                b.Z,
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
		Queue:            b.queue,
		BlockABuf:        b.blockABuf,
		GA:               b.gA,
		GB:               b.gB,
		GC:               b.gC,
		Readback:         b.readback,
		Context:          b.context,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		X:                b.X,
		Y:                b.Y,
		Z:                b.Z,
		MatrixA:          b.MatrixA,
		MatrixB:          b.MatrixB,
		MatrixC:          b.MatrixC,
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
	b.queue = s.Queue
	b.blockABuf = s.BlockABuf
	b.gA = s.GA
	b.gB = s.GB
	b.gC = s.GC
	b.readback = s.Readback
	b.context = s.Context
	b.gpus = s.Gpus
	b.Arch = s.Arch
	b.X = s.X
	b.Y = s.Y
	b.Z = s.Z
	b.MatrixA = s.MatrixA
	b.MatrixB = s.MatrixB
	b.MatrixC = s.MatrixC
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadKernel()
	return nil
}
