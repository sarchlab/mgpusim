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
	IsFetching                        bool
	SIMDID                            int
	SRegOffset                        int
	VRegOffset                        int
	LDSOffset                         int
	PC                                uint64
	EXEC                              uint64
	VCC                               uint64
	M0                                uint32
	SCC                               byte
	OutstandingScalarMemAccess        int
	OutstandingVectorMemAccess        int
	InFlightInsts                     int
}
