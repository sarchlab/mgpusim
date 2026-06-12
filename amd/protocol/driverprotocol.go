package protocol

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
)

// FlushReq requests the GPU to flush all the cache to the main memory.
type FlushReq struct {
	messaging.MsgMeta
}

// A LaunchKernelReq is a request that asks a GPU to launch a kernel.
type LaunchKernelReq struct {
	messaging.MsgMeta

	PID vm.PID

	Packet        *kernels.HsaKernelDispatchPacket
	PacketAddress uint64
	CodeObject    *insts.KernelCodeObject
	WGFilter      kernels.WGFilterFunc
}

// LaunchKernelRsp is the response that is sent by the GPU to the driver when
// the kernel completes execution. The original LaunchKernelReq is identified
// by MsgMeta.RspTo.
type LaunchKernelRsp struct {
	messaging.MsgMeta
}

// A MemCopyH2DReq is a request that asks the DMAEngine to copy memory from
// the host to the device.
type MemCopyH2DReq struct {
	messaging.MsgMeta
	SrcBuffer  []byte
	DstAddress uint64
}

// A MemCopyD2HReq is a request that asks the DMAEngine to copy memory from
// the device to the host.
type MemCopyD2HReq struct {
	messaging.MsgMeta
	SrcAddress uint64
	DstBuffer  []byte
}

// ShootDownCommand requests the GPU to perform a TLB shootdown and invalidate
// the corresponding PTEs.
type ShootDownCommand struct {
	messaging.MsgMeta

	VAddr []uint64
	PID   vm.PID
}

// ShootDownCompleteRsp indicates the completion of a TLB shootdown.
type ShootDownCompleteRsp struct {
	messaging.MsgMeta
}

// RDMADrainCmdFromDriver is the driver asking CP to drain the local RDMA.
type RDMADrainCmdFromDriver struct {
	messaging.MsgMeta
}

// RDMADrainRspToDriver is a rsp to driver indicating completion of RDMA
// drain.
type RDMADrainRspToDriver struct {
	messaging.MsgMeta
}

// RDMARestartCmdFromDriver is a cmd to unpause the RDMA.
type RDMARestartCmdFromDriver struct {
	messaging.MsgMeta
}

// RDMARestartRspToDriver indicates the RDMA restart is complete.
type RDMARestartRspToDriver struct {
	messaging.MsgMeta
}

// GPURestartReq is a req to GPU to start the pipeline and unpause all paused
// components.
type GPURestartReq struct {
	messaging.MsgMeta
}

// GPURestartRsp is a rsp indicating the restart is complete.
type GPURestartRsp struct {
	messaging.MsgMeta
}

// GeneralRsp is a generic acknowledgement that a request has completed. The
// request being acknowledged is identified by MsgMeta.RspTo. It replaces
// Akita v4's sim.GeneralRsp.
type GeneralRsp struct {
	messaging.MsgMeta
}
