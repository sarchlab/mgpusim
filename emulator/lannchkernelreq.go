package emulator

import (
	"gitlab.com/yaotsu/core/conn"
)

// A LaunchKernelReq is a request that asks a GPU to launch a kernel
type LaunchKernelReq struct {
	*conn.BasicRequest

	Packet *HsaKernelDispatchPacket
}

// NewLaunchKernelReq returns a new LaunchKernelReq
func NewLaunchKernelReq() *LaunchKernelReq {
	r := new(LaunchKernelReq)
	r.BasicRequest = conn.NewBasicRequest()
	return r
}
