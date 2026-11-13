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

// The page-migration commands of the driver. Every command names one page of
// the system page size and has a fixed size. A command processor works on
// the commands of different pages at the same time and answers each one with
// a GeneralRsp when its own work is done.

// PageHoldReq asks a command processor to hold the page at VAddr of process
// PID in every address translator of its GPU (memcontrolprotocol.CmdHold).
// The answer follows the last acknowledgement: no access of the GPU to the
// page is outstanding and no translation of it is in flight.
type PageHoldReq struct {
	PID      vm.PID
	VAddr    uint64
	PageSize uint64
}

// PageReleaseReq asks a command processor to release the page at VAddr of
// process PID in every address translator of its GPU
// (memcontrolprotocol.CmdRelease). The answer follows the last
// acknowledgement.
type PageReleaseReq struct {
	PID      vm.PID
	VAddr    uint64
	PageSize uint64
}

// PageShootdownReq asks a command processor to drop the translation of the
// page at VAddr of process PID from every TLB of its GPU.
type PageShootdownReq struct {
	PID      vm.PID
	VAddr    uint64
	PageSize uint64
}

// PageFlushReq asks the command processor of the GPU that a page leaves to
// write the dirty L2 lines of the frame at PAddr back and then drop the
// lines of the frame: on every L2 bank, a Flush of the frame, and, after its
// acknowledgement, an Invalidate. The answer follows the last
// acknowledgement, so the memory below the L2 holds the page.
type PageFlushReq struct {
	PAddr    uint64
	PageSize uint64
}

// PageInvalidateReq asks a command processor to drop the L2 lines of the
// frame at PAddr without writing them back. The driver sends it for the
// frame that a page moves to, which may have been used before, and waits
// for the answer before the page is copied: a line of the frame that the
// L2 was already writing back has then reached the memory below it.
type PageInvalidateReq struct {
	PAddr    uint64
	PageSize uint64
}

// PageCopyReq asks the command processor of the GPU that a page moves to to
// pull the page with its data mover: from SrcAddress, in the memory below
// the L2 of another GPU, to DstAddress, in the memory below its own L2. A
// command processor passes the requests to the data mover in the order they
// arrive.
type PageCopyReq struct {
	SrcAddress uint64
	DstAddress uint64
	PageSize   uint64
}

// AccessCounterNotification tells the driver that the remote accesses of GPU
// DeviceID to the physical page frame at PAddr reached the threshold of its
// RDMA engine (sarchlab/akita#590). The driver does not answer.
type AccessCounterNotification struct {
	PAddr    uint64
	DeviceID uint64
}
