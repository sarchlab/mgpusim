package protocol

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
)

// A CUPipelineRestartReq is a message from CP to ask the CU pipeline to resume
// after a flush/drain.
type CUPipelineRestartReq struct {
	messaging.MsgMeta
}

// A CUPipelineRestartRsp is a message from CU indicating the restart is
// complete.
type CUPipelineRestartRsp struct {
	messaging.MsgMeta
}

// A CUPipelineFlushReq is a message from CP to ask the CU pipeline to flush.
type CUPipelineFlushReq struct {
	messaging.MsgMeta
}

// A CUPipelineFlushRsp is a message from CU to CP indicating flush is
// complete.
type CUPipelineFlushRsp struct {
	messaging.MsgMeta
}

// WfDispatchLocation records the information about where to place the
// wavefront in a compute unit.
type WfDispatchLocation struct {
	Wavefront  *kernels.Wavefront
	SIMDID     int
	VGPROffset int
	SGPROffset int
	LDSOffset  int
}

// MapWGReq is a request that dispatches a work-group to a compute unit.
type MapWGReq struct {
	messaging.MsgMeta
	WorkGroup  *kernels.WorkGroup
	PID        vm.PID
	Wavefronts []WfDispatchLocation
}

// WGCompletionMsg notifies the dispatcher that work-groups have completed
// execution. RspToIDs carries the IDs of the MapWGReqs being responded to;
// it can acknowledge multiple MapWGReqs at once, hence a separate field
// rather than MsgMeta.RspTo.
type WGCompletionMsg struct {
	messaging.MsgMeta
	RspToIDs []uint64
}
