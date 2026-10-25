// Package benchmarks defines explicit, checkpointable host workloads.
package benchmarks

import (
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"io"
)

// Benchmark persists its continuation before each Step returns.
type Benchmark interface {
	Name() string
	SetName(string)
	Step() error
	IsEnded() bool
	SaveCheckpoint(io.Writer) error
	LoadCheckpoint(io.Reader) error
	SelectGPU([]int)
	SetUnifiedMemory()
	SetVerification(bool)
}

// Progress is the small common part of a benchmark's own saved continuation.
type Progress struct {
	ResourceName string
	Phase        int
	Ended        bool
	Verification bool
}

func (p *Progress) Name() string           { return p.ResourceName }
func (p *Progress) SetName(n string)       { p.ResourceName = n }
func (p *Progress) IsEnded() bool          { return p.Ended }
func (p *Progress) SetVerification(v bool) { p.Verification = v }

// Readback is a saved output-copy continuation with a benchmark-local target index.
type Readback struct {
	Source  driver.Ptr
	Command driver.CommandID
	Target  int
}
