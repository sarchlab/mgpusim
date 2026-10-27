package protocol

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
)

// WfDispatchLocation records the information about where to place the
// wavefront in a compute unit.
type WfDispatchLocation struct {
	WavefrontID uint64
	SIMDID      int
	VGPROffset  int
	SGPROffset  int
	LDSOffset   int
}

// MapWGReq is a request that dispatches a work-group to a compute unit.
type MapWGReq struct {
	WorkGroup  kernels.WorkGroupValue
	PID        vm.PID
	Wavefronts []WfDispatchLocation
}

// WGCompletionMsg notifies the dispatcher that work-groups have completed
// execution. RspToIDs carries the IDs of the MapWGReqs being responded to;
// it can acknowledge multiple MapWGReqs at once, hence a separate field
// rather than Msg.RspTo.
type WGCompletionMsg struct {
	RspToIDs []uint64
	Timings  []WavefrontTiming
}

// WavefrontTiming replaces sampling information formerly shared by pointer.
type WavefrontTiming struct {
	WavefrontID           uint64
	IssueTime, FinishTime timing.VTimeInPicoSec
}
