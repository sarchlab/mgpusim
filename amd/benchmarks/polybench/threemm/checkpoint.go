package threemm

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type State struct {
	benchmarks.Progress
	Readbacks []driver.HostBuffer
	Context   driver.Context
	Queue     driver.CommandQueue
	Gpus      []int
	Arch      arch.Type
	// Matrix dimensions. All default to N (set via NewBenchmark caller).
	NI               int
	NJ               int
	NK               int
	NL               int
	NM               int
	A                []float32 // NI×NK
	B                []float32 // NK×NJ
	C                []float32 // NJ×NM
	D                []float32 // NM×NL
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
		Gpus:             b.Gpus,
		Arch:             b.Arch,
		NI:               b.NI,
		NJ:               b.NJ,
		NK:               b.NK,
		NL:               b.NL,
		NM:               b.NM,
		UseUnifiedMemory: b.UseUnifiedMemory,
	}
}
func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
