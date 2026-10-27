package cu

import (
	"sort"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
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
	InstID    uint64
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
	InstID    uint64
}

func (v VectorMemAccessInfo) requestID() uint64 {
	if v.Read.ID != 0 {
		return v.Read.ID
	}
	return v.Write.ID
}

// Transactions are allocated and admitted in increasing request-ID order.
// Retirement preserves order, including after a checkpoint rewind.
func (cu *ComputeUnit) vectorTransactionIndex(id uint64) int {
	values := cu.comp.State.Vector
	index := sort.Search(len(values), func(i int) bool { return values[i].requestID() >= id })
	if index == len(values) || values[index].requestID() != id {
		return -1
	}
	return index
}
func (cu *ComputeUnit) vectorTransactionByID(id uint64) VectorAccessState {
	if index := cu.vectorTransactionIndex(id); index >= 0 {
		return cu.comp.State.Vector[index]
	}
	panic("cu: unknown vector transaction")
}

// admitVector enforces the binary-search invariant at the mutation boundary.
func (cu *ComputeUnit) admitVector(value VectorAccessState) {
	values := cu.comp.State.Vector
	id := value.requestID()
	if id == 0 || (len(values) > 0 && id <= values[len(values)-1].requestID()) {
		panic("cu: vector request IDs must increase on admission")
	}
	cu.comp.State.Vector = append(values, value)
}
