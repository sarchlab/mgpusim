package cu

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

type vectorMemAccessLaneInfo struct {
	laneID                int
	reg                   *insts.Reg
	regCount              int
	addrOffsetInCacheLine uint64
}

// VectorMemAccessInfo defines access info. Read and Write point at the
// request value so the coalescer and the vector memory unit can fill in the
// routing fields in place; exactly one of them is non-nil.
type VectorMemAccessInfo struct {
	Read      messaging.Msg
	Write     messaging.Msg
	Wavefront *Wavefront
	Inst      *wavefront.Inst
	laneInfo  []vectorMemAccessLaneInfo
}

// InstFetchReqInfo defines request info
type InstFetchReqInfo struct {
	Req       messaging.Msg
	Wavefront *Wavefront
	Address   uint64

	// FetchTaskID is the tracing task ID of the fetch task (v4 used the
	// string ID req.ID+"_fetch").
	FetchTaskID uint64
}

// ScalarMemAccessInfo defines request info
type ScalarMemAccessInfo struct {
	Req       messaging.Msg
	Wavefront *Wavefront
	DstSGPR   *insts.Reg
	Inst      *wavefront.Inst
}

func (v VectorMemAccessInfo) requestID() uint64 {
	if v.Read.ID != 0 {
		return v.Read.ID
	}
	return v.Write.ID
}
func (cu *ComputeUnit) vectorTransactionByID(id uint64) VectorAccessState {
	for _, v := range cu.comp.State.Vector {
		if v.requestID() == id {
			return v
		}
	}
	panic("cu: unknown vector transaction")
}
