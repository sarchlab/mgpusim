package rdma

import (
	"gitlab.com/akita/akita"
)

// A RDMADrainReq asks the rdma to stop processing requests from L1 while allowing pending requests to L2 to complete
type RDMADrainReq struct {
	akita.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *RDMADrainReq) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

// RDMADrainReqBuilder can build RDMA drain requests
type RDMADrainReqBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
}

// WithSendTime sets the send time of the request to build.:w
func (b RDMADrainReqBuilder) WithSendTime(
	t akita.VTimeInSec,
) RDMADrainReqBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b RDMADrainReqBuilder) WithSrc(src akita.Port) RDMADrainReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b RDMADrainReqBuilder) WithDst(dst akita.Port) RDMADrainReqBuilder {
	b.dst = dst
	return b
}

// Build creats a new RDMADrainReq
func (b RDMADrainReqBuilder) Build() *RDMADrainReq {
	r := &RDMADrainReq{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

// A RDMARestartReq is a message to allow rdma to continue processing reqs from L1
type RDMARestartReq struct {
	akita.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *RDMARestartReq) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

// RDMARestartBuilder can build RDMA restart req
type RDMARestartReqBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
}

// WithSendTime sets the send time of the request to build
func (b RDMARestartReqBuilder) WithSendTime(
	t akita.VTimeInSec,
) RDMARestartReqBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b RDMARestartReqBuilder) WithSrc(src akita.Port) RDMARestartReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b RDMARestartReqBuilder) WithDst(dst akita.Port) RDMARestartReqBuilder {
	b.dst = dst
	return b
}

// Build creats a new RDMADrainRsp
func (b RDMARestartReqBuilder) Build() *RDMARestartReq {
	r := &RDMARestartReq{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

// A RDMADrainRsp is a drain complete response to a RDMA Drain Req
type RDMADrainRsp struct {
	akita.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *RDMADrainRsp) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

// RDMADrainRspBuilder can build RDMA drain responses
type RDMADrainRspBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
}

// WithSendTime sets the send time of the request to build
func (b RDMADrainRspBuilder) WithSendTime(
	t akita.VTimeInSec,
) RDMADrainRspBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b RDMADrainRspBuilder) WithSrc(src akita.Port) RDMADrainRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b RDMADrainRspBuilder) WithDst(dst akita.Port) RDMADrainRspBuilder {
	b.dst = dst
	return b
}

// Build creats a new RDMADrainRsp
func (b RDMADrainRspBuilder) Build() *RDMADrainRsp {
	r := &RDMADrainRsp{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

// A RDMADrainRsp is a drain complete response to a RDMA Drain Req
type RDMARestartRsp struct {
	akita.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *RDMARestartRsp) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

// RDMADrainRspBuilder can build RDMA drain responses
type RDMARestartRspBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
}

// WithSendTime sets the send time of the request to build
func (b RDMARestartRspBuilder) WithSendTime(
	t akita.VTimeInSec,
) RDMARestartRspBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b RDMARestartRspBuilder) WithSrc(src akita.Port) RDMARestartRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b RDMARestartRspBuilder) WithDst(dst akita.Port) RDMARestartRspBuilder {
	b.dst = dst
	return b
}

// Build creats a new RDMADrainRsp
func (b RDMARestartRspBuilder) Build() *RDMARestartRsp {
	r := &RDMARestartRsp{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}
