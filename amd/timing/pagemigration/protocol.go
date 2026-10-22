package pagemigration

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
)

const (
	pullReqBytes    = 12 // same overhead as a memory read request
	pullRspOverhead = 4  // same overhead as a data-ready response
	ctrlMsgBytes    = 4
)

// PullReq asks the engine of the source GPU for Size bytes at PAddr. It is
// sent by the engine of the destination GPU.
type PullReq struct {
	messaging.MsgMeta

	PAddr uint64
	Size  uint64
}

// PullRsp carries the data of a PullReq back to the destination GPU. The
// PullReq is identified by MsgMeta.RspTo.
type PullRsp struct {
	messaging.MsgMeta

	Data []byte
}

// CopyReq asks the engine of the destination GPU to copy Size bytes from
// SrcPAddr, on the GPU whose engine is Source, to DstPAddr in its own
// memory. It is sent by the manager.
type CopyReq struct {
	messaging.MsgMeta

	Source   messaging.RemotePort
	SrcPAddr uint64
	DstPAddr uint64
	Size     uint64
}

// CopyRsp reports that a CopyReq, identified by MsgMeta.RspTo, has been
// written to the destination memory.
type CopyRsp struct {
	messaging.MsgMeta
}

// InvalidateReq asks the engine of a GPU to drop the translations of the
// pages at VAddrs of process PID from every TLB of the GPU. It is sent by the
// manager when a migration starts.
type InvalidateReq struct {
	messaging.MsgMeta

	PID    vm.PID
	VAddrs []uint64
}

// InvalidateRsp reports that every TLB of the GPU has dropped the pages of
// an InvalidateReq, identified by MsgMeta.RspTo.
type InvalidateRsp struct {
	messaging.MsgMeta
}
