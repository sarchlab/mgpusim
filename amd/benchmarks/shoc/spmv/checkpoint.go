package spmv

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/matrix/csr"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress

	Readbacks        []benchmarks.Readback
	Queue            driver.CommandQueue
	Context          driver.Context
	Gpus             []int
	Queues           []driver.CommandQueue
	UseUnifiedMemory bool
	Arch             arch.Type
	Dim              int32
	Sparsity         float64
	DValData         driver.Ptr
	DVecData         driver.Ptr
	DColsData        driver.Ptr
	DRowDData        driver.Ptr
	DOutData         driver.Ptr
	NItems           int32
	Vec              []float32
	Out              []float32
	Maxval           float32
	Matrix           csr.Matrix
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	UseUnifiedMemory bool
	Arch             arch.Type
	Dim              int32
	Sparsity         float64
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.Gpus,
		UseUnifiedMemory: b.UseUnifiedMemory,
		Arch:             b.Arch,
		Dim:              b.Dim,
		Sparsity:         b.Sparsity,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
