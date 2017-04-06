package cu

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
)

// A Scheduler is responsible for determine which wavefront can fetch, decode,
// and issue
//
//    <=> ToInstMem The port to the instruction memory unit
type Scheduler struct {
	*core.BasicComponent

	InstMem core.Component
}

// FetchInfo keeps record of the information of a fetch action
type FetchInfo struct {
	Buf []byte
	Wf  *Wavefront
}

// A Wavefront in the timing package contains the information of the progress
// of a wavefront
type Wavefront struct {
	*kernels.Wavefront

	PC uint64
}

// A WavefrontPool holds the wavefronts that will be scheduled in one SIMD
// unit
type WavefrontPool struct {
	capacity int

	Wfs         []*Wavefront
	FetchBuffer []*FetchInfo
}

// A FetchArbitrator can decide which wavefront in a scheduler can fetch
// instructions
type FetchArbitrator interface {
}

// An IssueArbitrator decides which wavefront can issue instruction
type IssueArbitrator interface {
}
