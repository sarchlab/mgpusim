package mgpusim

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/kernels"
	"gitlab.com/akita/util/ca"
)

//A CUPipelineRestartReq is a message from CP to ask the CU pipeline to resume after a flush/drain
type CUPipelineRestartReq struct {
	akita.MsgMeta
}

// Meta returns the meta data associated with the message.
func (m *CUPipelineRestartReq) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// CUPipelineRestartReqBuilder can build new CU restart reqs
type CUPipelineRestartReqBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
}

// WithSendTime sets the send time of the request to build.:w
func (b CUPipelineRestartReqBuilder) WithSendTime(
	t akita.VTimeInSec,
) CUPipelineRestartReqBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b CUPipelineRestartReqBuilder) WithSrc(src akita.Port) CUPipelineRestartReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b CUPipelineRestartReqBuilder) WithDst(dst akita.Port) CUPipelineRestartReqBuilder {
	b.dst = dst
	return b
}

// Build creats a new CUPipelineRestartReq
func (b CUPipelineRestartReqBuilder) Build() *CUPipelineRestartReq {
	r := &CUPipelineRestartReq{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

//A CUPipelineRestartRsp is a message from CU indicating the restart is complete
type CUPipelineRestartRsp struct {
	akita.MsgMeta
}

// Meta returns the meta data associated with the message.
func (m *CUPipelineRestartRsp) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// CUPipelineRestartReqBuilder can build new CU restart reqs
type CUPipelineRestartRspBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
}

// WithSendTime sets the send time of the request to build.:w
func (b CUPipelineRestartRspBuilder) WithSendTime(
	t akita.VTimeInSec,
) CUPipelineRestartRspBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b CUPipelineRestartRspBuilder) WithSrc(src akita.Port) CUPipelineRestartRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b CUPipelineRestartRspBuilder) WithDst(dst akita.Port) CUPipelineRestartRspBuilder {
	b.dst = dst
	return b
}

// Build creats a new CUPipelineRestartRsp
func (b CUPipelineRestartRspBuilder) Build() *CUPipelineRestartRsp {
	r := &CUPipelineRestartRsp{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

//A CUPipelineFlushReq is a message from CP to ask the CU pipeline to flush
type CUPipelineFlushReq struct {
	akita.MsgMeta
}

// Meta returns the meta data associated with the message.
func (m *CUPipelineFlushReq) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// CUPipelineFlushReqBuilder can build new CU flush reqs
type CUPipelineFlushReqBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
}

// WithSendTime sets the send time of the request to build.:w
func (b CUPipelineFlushReqBuilder) WithSendTime(
	t akita.VTimeInSec,
) CUPipelineFlushReqBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b CUPipelineFlushReqBuilder) WithSrc(src akita.Port) CUPipelineFlushReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b CUPipelineFlushReqBuilder) WithDst(dst akita.Port) CUPipelineFlushReqBuilder {
	b.dst = dst
	return b
}

// Build creats a new CUPipelineFlushReq
func (b CUPipelineFlushReqBuilder) Build() *CUPipelineFlushReq {
	r := &CUPipelineFlushReq{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

//A CUPipelineFlushRsp is a message from CU to CP indicating flush is complete
type CUPipelineFlushRsp struct {
	akita.MsgMeta
}

// Meta returns the meta data associated with the message.
func (m *CUPipelineFlushRsp) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// CUPipelineFlushRspBuilder can build new CU flush rsps
type CUPipelineFlushRspBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
}

// WithSendTime sets the send time of the request to build.:w
func (b CUPipelineFlushRspBuilder) WithSendTime(
	t akita.VTimeInSec,
) CUPipelineFlushRspBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b CUPipelineFlushRspBuilder) WithSrc(src akita.Port) CUPipelineFlushRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b CUPipelineFlushRspBuilder) WithDst(dst akita.Port) CUPipelineFlushRspBuilder {
	b.dst = dst
	return b
}

// Build creates a new CUPipelineFlushRsp
func (b CUPipelineFlushRspBuilder) Build() *CUPipelineFlushRsp {
	r := &CUPipelineFlushRsp{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

// MapWGReq is a request that is send by the Dispatcher to a ComputeUnit to
// ask the ComputeUnit to reserve resources for the work-group
type MapWGReq struct {
	akita.MsgMeta

	WG               *kernels.WorkGroup
	PID              ca.PID
	Ok               bool
	CUOutOfResources bool
}

// Meta returns the meta data associated with the message.
func (m *MapWGReq) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewMapWGReq returns a newly created MapWGReq
func NewMapWGReq(
	src, dst akita.Port,
	time akita.VTimeInSec,
	wg *kernels.WorkGroup,
) *MapWGReq {
	r := new(MapWGReq)
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = src
	r.Dst = dst
	r.SendTime = time
	r.WG = wg
	return r
}

// A WGFinishMsg is sent by a compute unit to notify about the completion of
// a work-group
type WGFinishMsg struct {
	akita.MsgMeta

	WG *kernels.WorkGroup
}

// Meta returns the meta data associated with the message.
func (m *WGFinishMsg) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewWGFinishMesg creates and returns a newly created WGFinishMsg
func NewWGFinishMesg(
	src, dst akita.Port,
	time akita.VTimeInSec,
	wg *kernels.WorkGroup,
) *WGFinishMsg {
	m := new(WGFinishMsg)
	m.ID = akita.GetIDGenerator().Generate()
	m.Src = src
	m.Dst = dst
	m.SendTime = time
	m.WG = wg

	return m
}
