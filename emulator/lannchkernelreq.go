package emulator

import (
	"gitlab.com/yaotsu/core/conn"
)

// A LaunchKernelReq is a request that asks a GPU to launch a kernel
type LaunchKernelReq struct {
	*conn.BasicRequest

	packet *HsaKernelDispatchPacket
}
