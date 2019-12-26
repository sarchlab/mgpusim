package timing

import (
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/timing/wavefront"
	"gitlab.com/akita/mem"
)

type vectorMemAccessLaneInfo struct {
	laneID                int
	reg                   *insts.Reg
	regCount              int
	addrOffsetInCacheLine uint64
}

type VectorMemAccessInfo struct {
	Read      *mem.ReadReq
	Write     *mem.WriteReq
	Wavefront *wavefront.Wavefront
	Inst      *wavefront.Inst
	laneInfo  []vectorMemAccessLaneInfo
}

type InstFetchReqInfo struct {
	Req       *mem.ReadReq
	Wavefront *wavefront.Wavefront
	Address   uint64
}

type ScalarMemAccessInfo struct {
	Req       *mem.ReadReq
	Wavefront *wavefront.Wavefront
	DstSGPR   *insts.Reg
	Inst      *wavefront.Inst
}
