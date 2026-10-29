package rdma

import (
	"github.com/sarchlab/akita/v5/messaging"
)

// AccessCounterNotification reports that GPU DeviceID accessed the page at
// physical address PAddr remotely Count times since the last notification
// of the page.
type AccessCounterNotification struct {
	messaging.MsgMeta

	PAddr    uint64
	DeviceID uint64
	Count    uint32
}

// DrainReq asks the RDMA engine to stop processing requests from L1 while
// allowing pending requests to L2 to complete.
type DrainReq struct {
	messaging.MsgMeta
}

// DrainRsp is a drain-complete response to a DrainReq. The original DrainReq
// is identified by MsgMeta.RspTo.
type DrainRsp struct {
	messaging.MsgMeta
}

// RestartReq is a message that allows the RDMA engine to continue processing
// requests from L1.
type RestartReq struct {
	messaging.MsgMeta
}

// RestartRsp is a restart-complete response to a RestartReq. The original
// RestartReq is identified by MsgMeta.RspTo.
type RestartRsp struct {
	messaging.MsgMeta
}
