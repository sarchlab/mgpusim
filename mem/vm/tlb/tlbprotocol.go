package tlb

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/vm"
)

// A FlushReq asks the TLB to invalidate certain entries. It will also not block all incoming and outgoing ports
type FlushReq struct {
	sim.MsgMeta
	VAddr []uint64
	PID   vm.PID
}

// Meta returns the meta data associated with the message.
func (r *FlushReq) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// FlushReqBuilder can build AT flush requests
type FlushReqBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
	vAddrs   []uint64
	pid      vm.PID
}

// WithSendTime sets the send time of the request to build.:w
func (b FlushReqBuilder) WithSendTime(
	t sim.VTimeInSec,
) FlushReqBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b FlushReqBuilder) WithSrc(src sim.Port) FlushReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b FlushReqBuilder) WithDst(dst sim.Port) FlushReqBuilder {
	b.dst = dst
	return b
}

// WithVAddrs sets the Vaddr of the pages to be flushed
func (b FlushReqBuilder) WithVAddrs(vAddrs []uint64) FlushReqBuilder {
	b.vAddrs = vAddrs
	return b
}

// WithPID sets the pid whose entries are to be flushed
func (b FlushReqBuilder) WithPID(pid vm.PID) FlushReqBuilder {
	b.pid = pid
	return b
}

// Build creates a new TLBFlushReq
func (b FlushReqBuilder) Build() *FlushReq {
	r := &FlushReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	r.VAddr = b.vAddrs
	r.PID = b.pid
	return r
}

// A FlushRsp is a response from AT indicating flush is complete
type FlushRsp struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *FlushRsp) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// FlushRspBuilder can build AT flush rsp
type FlushRspBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
}

// WithSendTime sets the send time of the request to build.:w
func (b FlushRspBuilder) WithSendTime(
	t sim.VTimeInSec,
) FlushRspBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b FlushRspBuilder) WithSrc(src sim.Port) FlushRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b FlushRspBuilder) WithDst(dst sim.Port) FlushRspBuilder {
	b.dst = dst
	return b
}

// Build creates a new TLBFlushRsps.
func (b FlushRspBuilder) Build() *FlushRsp {
	r := &FlushRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime

	return r
}

// A RestartReq is a request to TLB to start accepting requests and resume operations
type RestartReq struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *RestartReq) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// RestartReqBuilder can build TLB restart requests.
type RestartReqBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
}

// WithSendTime sets the send time of the request to build.
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

// Build creates a new TLBRestartReq.
func (b RestartReqBuilder) Build() *RestartReq {
	r := &RestartReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime

	return r
}

// A RestartRsp is a response from AT indicating it has resumed working
type RestartRsp struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *RestartRsp) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// RestartRspBuilder can build AT flush rsp
type RestartRspBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
}

// WithSendTime sets the send time of the request to build.:w
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

// Build creates a new TLBRestartRsp
func (b RestartRspBuilder) Build() *RestartRsp {
	r := &RestartRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime

	return r
}
