package cu

import (
	"github.com/sarchlab/akita/v5/mem/memprotocol"
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
	Read      *memprotocol.ReadReq
	Write     *memprotocol.WriteReq
	Wavefront *wavefront.Wavefront
	Inst      *wavefront.Inst
	laneInfo  []vectorMemAccessLaneInfo
}

// InstFetchReqInfo defines request info
type InstFetchReqInfo struct {
	Req       memprotocol.ReadReq
	Wavefront *wavefront.Wavefront
	Address   uint64

	// FetchTaskID is the tracing task ID of the fetch task (v4 used the
	// string ID req.ID+"_fetch").
	FetchTaskID uint64
}

// ScalarMemAccessInfo defines request info
type ScalarMemAccessInfo struct {
	Req       memprotocol.ReadReq
	Wavefront *wavefront.Wavefront
	DstSGPR   *insts.Reg
	Inst      *wavefront.Inst
}
