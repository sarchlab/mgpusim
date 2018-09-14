package timing

import "gitlab.com/akita/mem"

type VectorMemAccessInfo struct {
	Read      *mem.ReadReq
	Write     *mem.WriteReq
	Wavefront *Wavefront
	Lanes     []int
}

type InstFetchReqInfo struct {
	Req       *mem.ReadReq
	Wavefront *Wavefront
	Address   uint64
}

type ScalarMemAccessInfo struct {
	Req       *mem.ReadReq
	Wavefront *Wavefront
}
