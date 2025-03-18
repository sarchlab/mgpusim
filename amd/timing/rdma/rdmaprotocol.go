package rdma

import (
	"github.com/sarchlab/akita/v4/sim"
)

// DrainReq asks the rdma to stop processing requests from L1 while allowing pending requests to L2 to complete
type DrainReq struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *DrainReq) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// Clone returns a clone of the DrainReq with different ID.
func (r *DrainReq) Clone() sim.Msg {
	cloneMsg := *r
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// DrainReqBuilder can build RDMA drain requests
type DrainReqBuilder struct {
	src, dst sim.RemotePort
}

// WithSrc sets the source of the request to build.
func (b DrainReqBuilder) WithSrc(src sim.RemotePort) DrainReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b DrainReqBuilder) WithDst(dst sim.RemotePort) DrainReqBuilder {
	b.dst = dst
	return b
}

// Build creats a new DrainReq
func (b DrainReqBuilder) Build() *DrainReq {
	r := &DrainReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
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

// Clone returns a clone of the RestartReq with different ID.
func (r *RestartReq) Clone() sim.Msg {
	cloneMsg := *r
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// RestartReqBuilder can build RDMA restart req
type RestartReqBuilder struct {
	src, dst sim.RemotePort
}

// WithSrc sets the source of the request to build.
func (b RestartReqBuilder) WithSrc(src sim.RemotePort) RestartReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b RestartReqBuilder) WithDst(dst sim.RemotePort) RestartReqBuilder {
	b.dst = dst
	return b
}

// Build creats a new RDMADrainRsp
func (b RestartReqBuilder) Build() *RestartReq {
	r := &RestartReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	return r
}

// DrainRsp is a drain complete response to a RDMA Drain Req
type DrainRsp struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *DrainRsp) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// Clone returns a clone of the DrainRsp with different ID.
func (r *DrainRsp) Clone() sim.Msg {
	cloneMsg := *r
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// DrainRspBuilder can build RDMA drain responses
type DrainRspBuilder struct {
	src, dst sim.RemotePort
}

// WithSrc sets the source of the request to build.
func (b DrainRspBuilder) WithSrc(src sim.RemotePort) DrainRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b DrainRspBuilder) WithDst(dst sim.RemotePort) DrainRspBuilder {
	b.dst = dst
	return b
}

// Build creats a new RDMADrainRsp
func (b DrainRspBuilder) Build() *DrainRsp {
	r := &DrainRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	return r
}

// RestartRsp is a drain complete response to a RDMA Drain Req
type RestartRsp struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *RestartRsp) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// Clone returns a clone of the RestartRsp with different ID.
func (r *RestartRsp) Clone() sim.Msg {
	cloneMsg := *r
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// RestartRspBuilder can build RDMA drain responses
type RestartRspBuilder struct {
	src, dst sim.RemotePort
}

// WithSrc sets the source of the request to build.
func (b RestartRspBuilder) WithSrc(src sim.RemotePort) RestartRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b RestartRspBuilder) WithDst(dst sim.RemotePort) RestartRspBuilder {
	b.dst = dst
	return b
}

// Build creats a new RDMADrainRsp
func (b RestartRspBuilder) Build() *RestartRsp {
	r := &RestartRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	return r
}
