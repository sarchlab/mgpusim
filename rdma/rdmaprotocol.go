package rdma

import (
	"gitlab.com/akita/akita"
)

// DrainReq asks the rdma to stop processing requests from L1 while allowing pending requests to L2 to complete
type DrainReq struct {
	akita.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *DrainReq) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

// DrainReqBuilder can build RDMA drain requests
type DrainReqBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
}

// WithSendTime sets the send time of the request to build.:w
func (b DrainReqBuilder) WithSendTime(
	t akita.VTimeInSec,
) DrainReqBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b DrainReqBuilder) WithSrc(src akita.Port) DrainReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b DrainReqBuilder) WithDst(dst akita.Port) DrainReqBuilder {
	b.dst = dst
	return b
}

// Build creats a new DrainReq
func (b DrainReqBuilder) Build() *DrainReq {
	r := &DrainReq{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

// RestartReq is a message to allow rdma to continue processing reqs from L1
type RestartReq struct {
	akita.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *RestartReq) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

// RestartReqBuilder can build RDMA restart req
type RestartReqBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
}

// WithSendTime sets the send time of the request to build
func (b RestartReqBuilder) WithSendTime(
	t akita.VTimeInSec,
) RestartReqBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b RestartReqBuilder) WithSrc(src akita.Port) RestartReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b RestartReqBuilder) WithDst(dst akita.Port) RestartReqBuilder {
	b.dst = dst
	return b
}

// Build creats a new RDMADrainRsp
func (b RestartReqBuilder) Build() *RestartReq {
	r := &RestartReq{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

//DrainRsp is a drain complete response to a RDMA Drain Req
type DrainRsp struct {
	akita.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *DrainRsp) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

// DrainRspBuilder can build RDMA drain responses
type DrainRspBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
}

// WithSendTime sets the send time of the request to build
func (b DrainRspBuilder) WithSendTime(
	t akita.VTimeInSec,
) DrainRspBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b DrainRspBuilder) WithSrc(src akita.Port) DrainRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b DrainRspBuilder) WithDst(dst akita.Port) DrainRspBuilder {
	b.dst = dst
	return b
}

// Build creats a new RDMADrainRsp
func (b DrainRspBuilder) Build() *DrainRsp {
	r := &DrainRsp{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

//RestartRsp is a drain complete response to a RDMA Drain Req
type RestartRsp struct {
	akita.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *RestartRsp) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

//RestartRspBuilder can build RDMA drain responses
type RestartRspBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
}

// WithSendTime sets the send time of the request to build
func (b RestartRspBuilder) WithSendTime(
	t akita.VTimeInSec,
) RestartRspBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b RestartRspBuilder) WithSrc(src akita.Port) RestartRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b RestartRspBuilder) WithDst(dst akita.Port) RestartRspBuilder {
	b.dst = dst
	return b
}

// Build creats a new RDMADrainRsp
func (b RestartRspBuilder) Build() *RestartRsp {
	r := &RestartRsp{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}
