package protocol

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
)

// FlushReq requests the GPU to flush all the cache to the main memory.
type FlushReq struct {
}

// A LaunchKernelReq is a request that asks a GPU to launch a kernel.
type LaunchKernelReq struct {
	PID vm.PID

	Packet        kernels.HsaKernelDispatchPacket
	PacketAddress uint64
	CodeObject    insts.CodeObjectValue
	Selection     kernels.WGSelection
}

// LaunchKernelRsp is the response that is sent by the GPU to the driver when
// the kernel completes execution. The original LaunchKernelReq is identified
// by Msg.RspTo.
type LaunchKernelRsp struct {
}

// A MemCopyH2DReq is a request that asks the DMAEngine to copy memory from
// the host to the device.
type MemCopyH2DReq struct {
	SrcBuffer  []byte
	DstAddress uint64
}

// A MemCopyD2HReq is a request that asks the DMAEngine to copy memory from
// the device to the host.
type MemCopyD2HReq struct {
	SrcAddress uint64
	ByteSize   uint64
	DstOffset  uint64
}

// RDMADrainCmdFromDriver is the driver asking CP to drain the local RDMA.
type RDMADrainCmdFromDriver struct {
}

// RDMADrainRspToDriver is a rsp to driver indicating completion of RDMA
// drain.
type RDMADrainRspToDriver struct {
}

// RDMARestartCmdFromDriver is a cmd to unpause the RDMA.
type RDMARestartCmdFromDriver struct {
}

// RDMARestartRspToDriver indicates the RDMA restart is complete.
type RDMARestartRspToDriver struct {
}

// GeneralRsp is a generic acknowledgement that a request has completed. The
// request being acknowledged is identified by Msg.RspTo. It replaces
// Akita v4's sim.GeneralRsp.
type GeneralRsp struct {
}

// MemCopyD2HRsp returns copied bytes to the host driver. The request's destination
// offset is host bookkeeping; the GPU never mutates a caller-owned buffer.
type MemCopyD2HRsp struct{ Data []byte }
