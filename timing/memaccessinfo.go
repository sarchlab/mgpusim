package timing

import (
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/mem"
)

type VectorMemAccessInfo struct {
	Read            *mem.ReadReq
	Write           *mem.WriteReq
	Wavefront       *Wavefront
	DstVGPR         *insts.Reg
	RegisterCount   int
	Lanes           []int
	LaneAddrOffsets []uint64
	Inst            *Inst
}

type InstFetchReqInfo struct {
	Req       *mem.ReadReq
	Wavefront *Wavefront
	Address   uint64
}

type ScalarMemAccessInfo struct {
	Req       *mem.ReadReq
	Wavefront *Wavefront
	DstSGPR   *insts.Reg
	Inst      *Inst
}
