package cache

import (
	"github.com/rs/xid"
	"github.com/sarchlab/akita/v3/sim"
)

// FlushReq is the request send to a cache unit to request it to flush all
// the cache lines.
type FlushReq struct {
	sim.MsgMeta
	InvalidateAllCachelines bool
	DiscardInflight         bool
	PauseAfterFlushing      bool
}

// Meta returns the meta data associated with the message.
func (r *FlushReq) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// FlushReqBuilder can build flush requests.
type FlushReqBuilder struct {
	sendTime                sim.VTimeInSec
	src, dst                sim.Port
	invalidateAllCacheLines bool
	discardInflight         bool
	pauseAfterFlushing      bool
}

// WithSendTime sets the send time of the message to build.
func (b FlushReqBuilder) WithSendTime(t sim.VTimeInSec) FlushReqBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the message to build
func (b FlushReqBuilder) WithSrc(src sim.Port) FlushReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the message to build.
func (b FlushReqBuilder) WithDst(dst sim.Port) FlushReqBuilder {
	b.dst = dst
	return b
}

// InvalidateAllCacheLines allows the flush request to build to invalidate
// all the cachelines in a cache unit.
func (b FlushReqBuilder) InvalidateAllCacheLines() FlushReqBuilder {
	b.invalidateAllCacheLines = true
	return b
}

// DiscardInflight allows the flush request to build to discard all inflight
// requests.
func (b FlushReqBuilder) DiscardInflight() FlushReqBuilder {
	b.discardInflight = true
	return b
}

// PauseAfterFlushing sets the flush request to build to pause the cache unit
// from processing future request until restart request is received.
func (b FlushReqBuilder) PauseAfterFlushing() FlushReqBuilder {
	b.pauseAfterFlushing = true
	return b
}

// Build creates a new FlushReq
func (b FlushReqBuilder) Build() *FlushReq {
	r := &FlushReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	r.InvalidateAllCachelines = b.invalidateAllCacheLines
	r.DiscardInflight = b.discardInflight
	r.PauseAfterFlushing = b.pauseAfterFlushing
	return r
}

// FlushRsp is the respond sent from the a cache unit for finishing a cache
// flush
type FlushRsp struct {
	sim.MsgMeta
	RspTo string
}

// Meta returns the meta data associated with the message.
func (r *FlushRsp) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// FlushRspBuilder can build data ready responds.
type FlushRspBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
	rspTo    string
}

// WithSendTime sets the send time of the message to build.
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

// WithRspTo sets ID of the request that the respond to build is replying to.
func (b FlushRspBuilder) WithRspTo(id string) FlushRspBuilder {
	b.rspTo = id
	return b
}

// Build creates a new FlushRsp
func (b FlushRspBuilder) Build() *FlushRsp {
	r := &FlushRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	r.RspTo = b.rspTo
	return r
}

// RestartReq is the request send to a cache unit to request it unpause the
// cache
type RestartReq struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *RestartReq) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// RestartReqBuilder can build data ready responds.
type RestartReqBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
}

// WithSendTime sets the send time of the message to build.
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

// Build creates a new RestartReq
func (b RestartReqBuilder) Build() *RestartReq {
	r := &RestartReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

// RestartRsp is the respond sent from the a cache unit for finishing a cache
// flush
type RestartRsp struct {
	sim.MsgMeta
	RspTo string
}

// Meta returns the meta data associated with the message.
func (r *RestartRsp) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// RestartRspBuilder can build data ready responds.
type RestartRspBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
	rspTo    string
}

// WithSendTime sets the send time of the message to build.
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

// WithRspTo sets ID of the request that the respond to build is replying to.
func (b RestartRspBuilder) WithRspTo(id string) RestartRspBuilder {
	b.rspTo = id
	return b
}

// Build creates a new RestartRsp
func (b RestartRspBuilder) Build() *RestartRsp {
	r := &RestartRsp{}
	r.ID = xid.New().String()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	r.RspTo = b.rspTo
	return r
}
