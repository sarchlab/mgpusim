package stencil2d

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
	WCenter          float32
	WCardinal        float32
	WDiagonal        float32
	HInput           []float32
	HOutput          []float32
	NumIteration     int
	HaloWidth        int
	DData1           driver.Ptr
	DData2           driver.Ptr
	CurrData         driver.Ptr
	NewData          driver.Ptr
	NumRows          int
	NumCols          int
	DataSize         int
	NumPaddedCols    int
	PaddedDataSize   int
	Pad              int
	LocalRows        int
	LocalCols        int
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	NumIteration     int
	NumRows          int
	NumCols          int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		NumIteration:     b.NumIteration,
		NumRows:          b.NumRows,
		NumCols:          b.NumCols,
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
		WCenter:          b.wCenter,
		WCardinal:        b.wCardinal,
		WDiagonal:        b.wDiagonal,
		HInput:           b.hInput,
		HOutput:          b.hOutput,
		NumIteration:     b.NumIteration,
		HaloWidth:        b.haloWidth,
		DData1:           b.dData1,
		DData2:           b.dData2,
		CurrData:         b.currData,
		NewData:          b.newData,
		NumRows:          b.NumRows,
		NumCols:          b.NumCols,
		DataSize:         b.dataSize,
		NumPaddedCols:    b.numPaddedCols,
		PaddedDataSize:   b.paddedDataSize,
		Pad:              b.pad,
		LocalRows:        b.localRows,
		LocalCols:        b.localCols,
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
	b.wCenter = s.WCenter
	b.wCardinal = s.WCardinal
	b.wDiagonal = s.WDiagonal
	b.hInput = s.HInput
	b.hOutput = s.HOutput
	b.NumIteration = s.NumIteration
	b.haloWidth = s.HaloWidth
	b.dData1 = s.DData1
	b.dData2 = s.DData2
	b.currData = s.CurrData
	b.newData = s.NewData
	b.NumRows = s.NumRows
	b.NumCols = s.NumCols
	b.dataSize = s.DataSize
	b.numPaddedCols = s.NumPaddedCols
	b.paddedDataSize = s.PaddedDataSize
	b.pad = s.Pad
	b.localRows = s.LocalRows
	b.localCols = s.LocalCols
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadKernels()
	return nil
}
