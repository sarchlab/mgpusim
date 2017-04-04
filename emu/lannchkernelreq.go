package emu

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
)

// A LaunchKernelReq is a request that asks a GPU to launch a kernel
type LaunchKernelReq struct {
	*core.BasicRequest

	Packet *HsaKernelDispatchPacket
	HsaCo  *insts.HsaCo
}

// NewLaunchKernelReq returns a new LaunchKernelReq
func NewLaunchKernelReq() *LaunchKernelReq {
	r := new(LaunchKernelReq)
	r.BasicRequest = core.NewBasicRequest()
	return r
}
