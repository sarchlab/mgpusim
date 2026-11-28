package wavefront

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// ExecutionState is a wavefront's canonical register/control state. The CU
// resolves group, instruction, scoreboard, and register-file access by ID.
type ExecutionState struct {
	InstID, ToIssueID                 uint64
	PID                               vm.PID
	State                             WfState
	LastFetchTime                     timing.VTimeInPicoSec
	SampleIssueTime, SampleFinishTime timing.VTimeInPicoSec
	CompletedLanes                    int
	InstBuffer                        []byte
	InstBufferStartPC                 uint64
	// IsFetching is true while any fetch of this wavefront is outstanding,
	// including fetches made obsolete by a taken branch.
	IsFetching bool
	// FetchEpoch numbers the fetch streams of the wavefront. A taken branch
	// starts a new stream; fetches of an earlier stream are dropped when they
	// return.
	FetchEpoch uint64
	// NextFetchPC is the address of the next line (or block, with block
	// fetch) to request in the current stream. Those between the end of the
	// instruction buffer and this address are in flight. Zero when the stream
	// has requested nothing yet.
	NextFetchPC uint64
	// IssuedEnd is, with block fetch, the address after the last instruction
	// the wavefront issued in the current fetch stream; the buffer blocks
	// before it are free. Zero when the stream has issued nothing yet.
	IssuedEnd uint64
	// BlockFetchNotBefore is, with block fetch, the earliest time at which
	// the wavefront may request its next instruction block.
	BlockFetchNotBefore timing.VTimeInPicoSec
	// SCCBranchNotBefore is the earliest time at which the wavefront may issue
	// a conditional branch on SCC, which waits after a scalar ALU instruction
	// (see the compute unit's Spec.SCCToBranchCycles).
	SCCBranchNotBefore timing.VTimeInPicoSec
	// PrefetchPC is the next line to read ahead into the instruction cache in
	// the current stream. Zero when the stream has prefetched nothing yet.
	PrefetchPC                 uint64
	SIMDID                     int
	SRegOffset                 int
	VRegOffset                 int
	LDSOffset                  int
	PC                         uint64
	EXEC                       uint64
	VCC                        uint64
	M0                         uint32
	SCC                        byte
	OutstandingScalarMemAccess int
	OutstandingVectorMemAccess int
	InFlightInsts              int
}
