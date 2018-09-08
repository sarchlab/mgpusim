package kernels

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/insts"
)

// A LaunchKernelReq is a request that asks a GPU to launch a kernel
type LaunchKernelReq struct {
	*akita.ReqBase

	Packet        *HsaKernelDispatchPacket
	PacketAddress uint64
	HsaCo         *insts.HsaCo

	OK bool
}

// NewLaunchKernelReq returns a new LaunchKernelReq
func NewLaunchKernelReq() *LaunchKernelReq {
	r := new(LaunchKernelReq)
	r.ReqBase = akita.NewReqBase()
	return r
}
