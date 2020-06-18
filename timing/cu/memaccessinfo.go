package cu

import (
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/timing/wavefront"
)

type vectorMemAccessLaneInfo struct {
	laneID                int
	reg                   *insts.Reg
	regCount              int
	addrOffsetInCacheLine uint64
}

// VectorMemAccessInfo defines access info
type VectorMemAccessInfo struct {
	ID        string
	Read      *mem.ReadReq
	Write     *mem.WriteReq
	Wavefront *wavefront.Wavefront
	Inst      *wavefront.Inst
	laneInfo  []vectorMemAccessLaneInfo
}

// TaskID returns the ID of the VectorMemAccess transaction
func (i VectorMemAccessInfo) TaskID() string {
	return i.ID
}

// InstFetchReqInfo defines request info
type InstFetchReqInfo struct {
	Req       *mem.ReadReq
	Wavefront *wavefront.Wavefront
	Address   uint64
}

// ScalarMemAccessInfo defines request info
type ScalarMemAccessInfo struct {
	Req       *mem.ReadReq
	Wavefront *wavefront.Wavefront
	DstSGPR   *insts.Reg
	Inst      *wavefront.Inst
}
