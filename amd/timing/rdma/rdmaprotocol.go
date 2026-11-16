package rdma

import "github.com/sarchlab/akita/v5/sim/messaging"

// DrainReq asks the RDMA engine to stop processing requests from L1 while
// allowing pending requests to L2 to complete.
type DrainReq struct {
}

// DrainRsp is a drain-complete response to a DrainReq. The original DrainReq
// is identified by Msg.RspTo.
type DrainRsp struct {
}

// RestartReq is a message that allows the RDMA engine to continue processing
// requests from L1.
type RestartReq struct {
}

// RestartRsp is a restart-complete response to a RestartReq. The original
// RestartReq is identified by Msg.RspTo.
type RestartRsp struct {
}

var Protocol = messaging.DefineProtocol(
	messaging.RoleDef{Name: "requester", Sends: []any{DrainReq{}, RestartReq{}}},
	messaging.RoleDef{Name: "responder", Sends: []any{DrainRsp{}, RestartRsp{}}},
)
