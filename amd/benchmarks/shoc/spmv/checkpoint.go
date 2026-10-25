package spmv

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/matrix/csr"
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
		Gpus:             b.gpus,
		UseUnifiedMemory: b.useUnifiedMemory,
		Arch:             b.Arch,
		Dim:              b.Dim,
		Sparsity:         b.Sparsity,
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
		Dim:              b.Dim,
		Sparsity:         b.Sparsity,
		DValData:         b.dValData,
		DVecData:         b.dVecData,
		DColsData:        b.dColsData,
		DRowDData:        b.dRowDData,
		DOutData:         b.dOutData,
		NItems:           b.nItems,
		Vec:              b.vec,
		Out:              b.out,
		Maxval:           b.maxval,
		Matrix:           b.matrix,
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
	b.Dim = s.Dim
	b.Sparsity = s.Sparsity
	b.dValData = s.DValData
	b.dVecData = s.DVecData
	b.dColsData = s.DColsData
	b.dRowDData = s.DRowDData
	b.dOutData = s.DOutData
	b.nItems = s.NItems
	b.vec = s.Vec
	b.out = s.Out
	b.maxval = s.Maxval
	b.matrix = s.Matrix
	b.loadProgram()
	return nil
}
