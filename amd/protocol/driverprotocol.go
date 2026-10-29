package protocol

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
)

// FlushReq requests the GPU to flush all the cache to the main memory.
type FlushReq struct {
}

// A LaunchKernelReq asks a GPU to launch an already-uploaded kernel.
// Executable bytes live at Packet.KernelObject in the PID address space; only
// launch metadata crosses the driver boundary.
type LaunchKernelReq struct {
	PID vm.PID

	Packet        kernels.HsaKernelDispatchPacket
	PacketAddress uint64
	CodeObject    insts.CodeObjectMetadata
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

// PageHoldReq asks a command processor to hold the pages at VAddrs of
// process PID in every address translator of its GPU
// (memcontrolprotocol.CmdHold). The command processor answers with a
// GeneralRsp once every translator has acknowledged: no access of the GPU to
// the pages is outstanding and no translation of them is in flight.
type PageHoldReq struct {
	PID    vm.PID
	VAddrs []uint64
}

// PageShootdownReq asks a command processor to drop the translations of the
// pages at VAddrs of process PID from every TLB of its GPU: Pause, then
// Invalidate, then Enable. It answers with a GeneralRsp.
type PageShootdownReq struct {
	PID    vm.PID
	VAddrs []uint64
}

// PageReleaseReq asks a command processor to release the pages at VAddrs of
// process PID in every address translator of its GPU
// (memcontrolprotocol.CmdRelease). It answers with a GeneralRsp.
type PageReleaseReq struct {
	PID    vm.PID
	VAddrs []uint64
}

// PageCopyReq asks the command processor of the destination GPU to copy
// ByteSize bytes from SrcAddress, in another GPU's memory, to DstAddress in
// its own memory with its data mover. It answers with a GeneralRsp.
type PageCopyReq struct {
	SrcAddress uint64
	DstAddress uint64
	ByteSize   uint64
}
