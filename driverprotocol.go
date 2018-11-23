package gcn3

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

// FlushCommand requests the GPU to flush all the cache to the main memory
type FlushCommand struct {
	*akita.ReqBase

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec
}

// NewFlushCommand Creates a new flush command, setting the request send time
// with time and the source and destination.
func NewFlushCommand(time akita.VTimeInSec, src, dst akita.Port) *FlushCommand {
	cmd := new(FlushCommand)
	cmd.ReqBase = akita.NewReqBase()
	cmd.SetSendTime(time)
	cmd.SetSrc(src)
	cmd.SetDst(dst)
	cmd.StartTime = time
	return cmd
}

// A LaunchKernelReq is a request that asks a GPU to launch a kernel
type LaunchKernelReq struct {
	*akita.ReqBase

	Packet        *kernels.HsaKernelDispatchPacket
	PacketAddress uint64
	HsaCo         *insts.HsaCo

	OK bool

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec
}

// NewLaunchKernelReq returns a new LaunchKernelReq
func NewLaunchKernelReq(time akita.VTimeInSec, src, dst akita.Port) *LaunchKernelReq {
	r := new(LaunchKernelReq)
	r.ReqBase = akita.NewReqBase()
	r.StartTime = time
	r.SetSrc(src)
	r.SetDst(dst)
	r.SetSendTime(time)
	return r
}

// A MemCopyH2DReq is a request that asks the DMAEngine to copy memory
// from the host to the device
type MemCopyH2DReq struct {
	*akita.ReqBase
	SrcBuffer  []byte
	DstAddress uint64

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec
}

// NewMemCopyH2DReq created a new MemCopyH2DReq
func NewMemCopyH2DReq(
	time akita.VTimeInSec,
	src, dst akita.Port,
	srcBuffer []byte,
	dstAddress uint64,
) *MemCopyH2DReq {
	reqBase := akita.NewReqBase()
	req := new(MemCopyH2DReq)
	req.ReqBase = reqBase
	req.SetSendTime(time)
	req.SetSrc(src)
	req.SetDst(dst)
	req.SrcBuffer = srcBuffer
	req.DstAddress = dstAddress
	req.StartTime = time
	return req
}

// A MemCopyD2HReq is a request that asks the DMAEngine to copy memory
// from the host to the device
type MemCopyD2HReq struct {
	*akita.ReqBase
	SrcAddress uint64
	DstBuffer  []byte

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec
}

// NewMemCopyD2HReq created a new MemCopyH2DReq
func NewMemCopyD2HReq(
	time akita.VTimeInSec,
	src, dst akita.Port,
	srcAddress uint64,
	dstBuffer []byte,
) *MemCopyD2HReq {
	reqBase := akita.NewReqBase()
	req := new(MemCopyD2HReq)
	req.ReqBase = reqBase
	req.SetSendTime(time)
	req.SetSrc(src)
	req.SetDst(dst)
	req.SrcAddress = srcAddress
	req.DstBuffer = dstBuffer
	return req
}
