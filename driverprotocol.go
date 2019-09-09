package gcn3

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
	"gitlab.com/akita/util/ca"
)

// FlushCommand requests the GPU to flush all the cache to the main memory
type FlushCommand struct {
	akita.MsgMeta
}

// Meta returns the meta data associated with the message.
func (m *FlushCommand) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// ShootDownCommand requests the GPU to perform a TLB shootdown and invalidate
// the corresponding PTE's
type ShootDownCommand struct {
	akita.MsgMeta

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec

	VAddr []uint64
	PID   ca.PID
}

// Meta returns the meta data associated with the message.
func (m *ShootDownCommand) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

type ShootDownCompleteRsp struct {
	akita.MsgMeta

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec

	shootDownComplete bool
}

// Meta returns the meta data associated with the message.
func (m *ShootDownCompleteRsp) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

//NewShootdownCommand tells the CP to drain all CU and invalidate PTE's in TLB and Page Tables
func NewShootdownCommand(
	time akita.VTimeInSec,
	src, dst akita.Port,
	vAddr []uint64,
	pID ca.PID,
) *ShootDownCommand {
	cmd := new(ShootDownCommand)
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	cmd.VAddr = vAddr
	cmd.PID = pID
	return cmd
}

func NewShootdownCompleteRsp(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *ShootDownCompleteRsp {
	cmd := new(ShootDownCompleteRsp)
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

// NewFlushCommand Creates a new flush command, setting the request send time
// with time and the source and destination.
func NewFlushCommand(time akita.VTimeInSec, src, dst akita.Port) *FlushCommand {
	cmd := new(FlushCommand)
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

// A LaunchKernelReq is a request that asks a GPU to launch a kernel
type LaunchKernelReq struct {
	akita.MsgMeta

	PID ca.PID

	Packet        *kernels.HsaKernelDispatchPacket
	PacketAddress uint64
	HsaCo         *insts.HsaCo

	OK bool
}

// Meta returns the meta data associated with the message.
func (m *LaunchKernelReq) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewLaunchKernelReq returns a new LaunchKernelReq
func NewLaunchKernelReq(
	time akita.VTimeInSec,
	src, dst akita.Port) *LaunchKernelReq {
	r := new(LaunchKernelReq)
	r.SendTime = time
	r.Src = src
	r.Dst = dst
	return r
}

// A MemCopyH2DReq is a request that asks the DMAEngine to copy memory
// from the host to the device
type MemCopyH2DReq struct {
	akita.MsgMeta
	SrcBuffer  []byte
	DstAddress uint64
}

// Meta returns the meta data associated with the message.
func (m *MemCopyH2DReq) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewMemCopyH2DReq created a new MemCopyH2DReq
func NewMemCopyH2DReq(
	time akita.VTimeInSec,
	src, dst akita.Port,
	srcBuffer []byte,
	dstAddress uint64,
) *MemCopyH2DReq {
	req := new(MemCopyH2DReq)
	req.SendTime = time
	req.Src = src
	req.Dst = dst
	req.SrcBuffer = srcBuffer
	req.DstAddress = dstAddress
	return req
}

// A MemCopyD2HReq is a request that asks the DMAEngine to copy memory
// from the host to the device
type MemCopyD2HReq struct {
	akita.MsgMeta
	SrcAddress uint64
	DstBuffer  []byte
}

// Meta returns the meta data associated with the message.
func (m *MemCopyD2HReq) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewMemCopyD2HReq created a new MemCopyH2DReq
func NewMemCopyD2HReq(
	time akita.VTimeInSec,
	src, dst akita.Port,
	srcAddress uint64,
	dstBuffer []byte,
) *MemCopyD2HReq {
	req := new(MemCopyD2HReq)
	req.SendTime = time
	req.Src = src
	req.Dst = dst
	req.SrcAddress = srcAddress
	req.DstBuffer = dstBuffer
	return req
}
