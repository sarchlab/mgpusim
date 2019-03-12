package timing

import (
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/timing/wavefront"
	"gitlab.com/akita/mem"
)

type VectorMemAccessInfo struct {
	Read            *mem.ReadReq
	Write           *mem.WriteReq
	Wavefront       *wavefront.Wavefront
	DstVGPR         *insts.Reg
	RegisterCount   int
	Lanes           []int
	LaneAddrOffsets []uint64
	Inst            *wavefront.Inst
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
