package rdma

import (
	"gitlab.com/akita/akita/v3/sim"
)

// DrainReq asks the rdma to stop processing requests from L1 while allowing pending requests to L2 to complete
type DrainReq struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *DrainReq) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// DrainReqBuilder can build RDMA drain requests
type DrainReqBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
}

// WithSendTime sets the send time of the request to build.:w
func (b DrainReqBuilder) WithSendTime(
	t sim.VTimeInSec,
) DrainReqBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b DrainReqBuilder) WithSrc(src sim.Port) DrainReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b DrainReqBuilder) WithDst(dst sim.Port) DrainReqBuilder {
	b.dst = dst
	return b
}

// Build creats a new DrainReq
func (b DrainReqBuilder) Build() *DrainReq {
	r := &DrainReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

// RestartReq is a message to allow rdma to continue processing reqs from L1
type RestartReq struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *RestartReq) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// RestartReqBuilder can build RDMA restart req
type RestartReqBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
}

// WithSendTime sets the send time of the request to build
func (b RestartReqBuilder) WithSendTime(
	t sim.VTimeInSec,
) RestartReqBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b RestartReqBuilder) WithSrc(src sim.Port) RestartReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b RestartReqBuilder) WithDst(dst sim.Port) RestartReqBuilder {
	b.dst = dst
	return b
}

// Build creats a new RDMADrainRsp
func (b RestartReqBuilder) Build() *RestartReq {
	r := &RestartReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

//DrainRsp is a drain complete response to a RDMA Drain Req
type DrainRsp struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *DrainRsp) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// DrainRspBuilder can build RDMA drain responses
type DrainRspBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
}

// WithSendTime sets the send time of the request to build
func (b DrainRspBuilder) WithSendTime(
	t sim.VTimeInSec,
) DrainRspBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b DrainRspBuilder) WithSrc(src sim.Port) DrainRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b DrainRspBuilder) WithDst(dst sim.Port) DrainRspBuilder {
	b.dst = dst
	return b
}

// Build creats a new RDMADrainRsp
func (b DrainRspBuilder) Build() *DrainRsp {
	r := &DrainRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

//RestartRsp is a drain complete response to a RDMA Drain Req
type RestartRsp struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *RestartRsp) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

//RestartRspBuilder can build RDMA drain responses
type RestartRspBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
}

// WithSendTime sets the send time of the request to build
func (b RestartRspBuilder) WithSendTime(
	t sim.VTimeInSec,
) RestartRspBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b RestartRspBuilder) WithSrc(src sim.Port) RestartRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b RestartRspBuilder) WithDst(dst sim.Port) RestartRspBuilder {
	b.dst = dst
	return b
}

// Build creats a new RDMADrainRsp
func (b RestartRspBuilder) Build() *RestartRsp {
	r := &RestartRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}
