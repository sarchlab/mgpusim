package stencil2d

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress

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
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		NumIteration:     b.NumIteration,
		NumRows:          b.NumRows,
		NumCols:          b.NumCols,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadKernels()
	return nil
}
