package pagemigration

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
)

const ctrlMsgBytes = 4

// HoldReq asks the command processor of a GPU to hold the pages at VAddrs of
// process PID in every address translator of the GPU
// (memcontrolprotocol.CmdHold). The command processor answers with a Rsp
// once every translator has acknowledged, i.e. once no access of the GPU to
// the pages and no translation of them is in flight.
type HoldReq struct {
	messaging.MsgMeta

	PID    vm.PID
	VAddrs []uint64
}

// ShootdownReq asks the command processor of a GPU to drop the translations
// of the pages at VAddrs of process PID from every TLB of the GPU: it
// pauses each TLB, invalidates the pages, and enables the TLB again. It
// answers with a Rsp once every TLB is enabled again.
type ShootdownReq struct {
	messaging.MsgMeta

	PID    vm.PID
	VAddrs []uint64
}

// ReleaseReq asks the command processor of a GPU to release the pages at
// VAddrs of process PID in every address translator of the GPU
// (memcontrolprotocol.CmdRelease): the accesses that wait on a Hold or a
// fault of the pages are translated again. It answers with a Rsp.
type ReleaseReq struct {
	messaging.MsgMeta

	PID    vm.PID
	VAddrs []uint64
}

// CopyReq asks the command processor of the destination GPU to copy Size
// bytes from SrcPAddr, on another GPU, to DstPAddr in its own memory, with
// its data mover. It answers with a Rsp once the data is written.
type CopyReq struct {
	messaging.MsgMeta

	SrcPAddr uint64
	DstPAddr uint64
	Size     uint64
}

// Rsp reports that the request identified by MsgMeta.RspTo is done.
type Rsp struct {
	messaging.MsgMeta
}
