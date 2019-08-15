package timing

import (
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/timing/wavefront"
	"gitlab.com/akita/mem"
)

<<<<<<< HEAD
type VectorMemAccessInfo struct {
	Read            *mem.ReadReq
	Write           *mem.WriteReq
	Wavefront       *wavefront.Wavefront
	DstVGPR         *insts.Reg
	RegisterCount   int
	Lanes           []int
	LaneAddrOffsets []uint64
	Inst            *wavefront.Inst
=======
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
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
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
