package protocol

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

// CUPipelineRestartRspBuilder can build new CU restart reqs
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

// WfDispatchLocation records the information about where to place the wavefront
// in a compute unit.
type WfDispatchLocation struct {
	Wavefront  *kernels.Wavefront
	SIMDID     int
	VGPROffset int
	SGPROffset int
	LDSOffset  int
}

// MapWGReq is a request that dispatches a work-group to a compute unit.
type MapWGReq struct {
	akita.MsgMeta
	WorkGroup  *kernels.WorkGroup
	PID        ca.PID
	Wavefronts []WfDispatchLocation
}

// Meta returns the meta data associated with the MapWGReq.
func (r *MapWGReq) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

// MapWGReqBuilder can build MapWGReqs.
type MapWGReqBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
	pid      ca.PID
	wg       *kernels.WorkGroup
	wfs      []WfDispatchLocation
}

// WithSendTime sets the send time.
func (b MapWGReqBuilder) WithSendTime(t akita.VTimeInSec) MapWGReqBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the message.
func (b MapWGReqBuilder) WithSrc(src akita.Port) MapWGReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the message.
func (b MapWGReqBuilder) WithDst(dst akita.Port) MapWGReqBuilder {
	b.dst = dst
	return b
}

// WithWG sets the work-group to dispatch.
func (b MapWGReqBuilder) WithWG(wg *kernels.WorkGroup) MapWGReqBuilder {
	b.wg = wg
	return b
}

// WithPID sets the PID of the work-group.
func (b MapWGReqBuilder) WithPID(pid ca.PID) MapWGReqBuilder {
	b.pid = pid
	return b
}

// AddWf adds the information to execute a wavefront.
func (b MapWGReqBuilder) AddWf(wf WfDispatchLocation) MapWGReqBuilder {
	b.wfs = append(b.wfs, wf)
	return b
}

// Build creates the MapWGReq.
func (b MapWGReqBuilder) Build() *MapWGReq {
	r := &MapWGReq{}
	r.Meta().ID = akita.GetIDGenerator().Generate()
	r.Meta().SendTime = b.sendTime
	r.Meta().Src = b.src
	r.Meta().Dst = b.dst
	r.PID = b.pid
	r.WorkGroup = b.wg
	r.Wavefronts = b.wfs
	return r
}

// WGCompletionMsg notifies the dispatcher that a work-group is completed
// execution
type WGCompletionMsg struct {
	akita.MsgMeta
	RspTo string
}

// Meta returns the meta data associated with the MapWGReq.
func (r *WGCompletionMsg) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

// WGCompletionMsgBuilder can build MapWGReqs.
type WGCompletionMsgBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
	rspTo    string
}

// WithSendTime sets the send time.
func (b WGCompletionMsgBuilder) WithSendTime(
	t akita.VTimeInSec,
) WGCompletionMsgBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the message.
func (b WGCompletionMsgBuilder) WithSrc(
	src akita.Port,
) WGCompletionMsgBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the message.
func (b WGCompletionMsgBuilder) WithDst(
	dst akita.Port,
) WGCompletionMsgBuilder {
	b.dst = dst
	return b
}

// WithRspTo sets rspTo
func (b WGCompletionMsgBuilder) WithRspTo(
	rspTo string,
) WGCompletionMsgBuilder {
	b.rspTo = rspTo
	return b
}

// Build builds WGCompletionMsg
func (b WGCompletionMsgBuilder) Build() *WGCompletionMsg {
	msg := &WGCompletionMsg{}
	msg.Meta().ID = akita.GetIDGenerator().Generate()
	msg.Meta().SendTime = b.sendTime
	msg.Meta().Src = b.src
	msg.Meta().Dst = b.dst
	msg.RspTo = b.rspTo
	return msg
}
