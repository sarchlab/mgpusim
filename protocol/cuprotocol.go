package protocol

import (
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v3/kernels"
)

// A CUPipelineRestartReq is a message from CP to ask the CU pipeline to resume after a flush/drain
type CUPipelineRestartReq struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (m *CUPipelineRestartReq) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// CUPipelineRestartReqBuilder can build new CU restart reqs
type CUPipelineRestartReqBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
}

// WithSendTime sets the send time of the request to build.:w
func (b CUPipelineRestartReqBuilder) WithSendTime(
	t sim.VTimeInSec,
) CUPipelineRestartReqBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b CUPipelineRestartReqBuilder) WithSrc(src sim.Port) CUPipelineRestartReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b CUPipelineRestartReqBuilder) WithDst(dst sim.Port) CUPipelineRestartReqBuilder {
	b.dst = dst
	return b
}

// Build creats a new CUPipelineRestartReq
func (b CUPipelineRestartReqBuilder) Build() *CUPipelineRestartReq {
	r := &CUPipelineRestartReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

// A CUPipelineRestartRsp is a message from CU indicating the restart is complete
type CUPipelineRestartRsp struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (m *CUPipelineRestartRsp) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// CUPipelineRestartRspBuilder can build new CU restart reqs
type CUPipelineRestartRspBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
}

// WithSendTime sets the send time of the request to build.:w
func (b CUPipelineRestartRspBuilder) WithSendTime(
	t sim.VTimeInSec,
) CUPipelineRestartRspBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b CUPipelineRestartRspBuilder) WithSrc(src sim.Port) CUPipelineRestartRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b CUPipelineRestartRspBuilder) WithDst(dst sim.Port) CUPipelineRestartRspBuilder {
	b.dst = dst
	return b
}

// Build creats a new CUPipelineRestartRsp
func (b CUPipelineRestartRspBuilder) Build() *CUPipelineRestartRsp {
	r := &CUPipelineRestartRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

// A CUPipelineFlushReq is a message from CP to ask the CU pipeline to flush
type CUPipelineFlushReq struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (m *CUPipelineFlushReq) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// CUPipelineFlushReqBuilder can build new CU flush reqs
type CUPipelineFlushReqBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
}

// WithSendTime sets the send time of the request to build.:w
func (b CUPipelineFlushReqBuilder) WithSendTime(
	t sim.VTimeInSec,
) CUPipelineFlushReqBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b CUPipelineFlushReqBuilder) WithSrc(src sim.Port) CUPipelineFlushReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b CUPipelineFlushReqBuilder) WithDst(dst sim.Port) CUPipelineFlushReqBuilder {
	b.dst = dst
	return b
}

// Build creats a new CUPipelineFlushReq
func (b CUPipelineFlushReqBuilder) Build() *CUPipelineFlushReq {
	r := &CUPipelineFlushReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	return r
}

// A CUPipelineFlushRsp is a message from CU to CP indicating flush is complete
type CUPipelineFlushRsp struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (m *CUPipelineFlushRsp) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// CUPipelineFlushRspBuilder can build new CU flush rsps
type CUPipelineFlushRspBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
}

// WithSendTime sets the send time of the request to build.:w
func (b CUPipelineFlushRspBuilder) WithSendTime(
	t sim.VTimeInSec,
) CUPipelineFlushRspBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b CUPipelineFlushRspBuilder) WithSrc(src sim.Port) CUPipelineFlushRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b CUPipelineFlushRspBuilder) WithDst(dst sim.Port) CUPipelineFlushRspBuilder {
	b.dst = dst
	return b
}

// Build creates a new CUPipelineFlushRsp
func (b CUPipelineFlushRspBuilder) Build() *CUPipelineFlushRsp {
	r := &CUPipelineFlushRsp{}
	r.ID = sim.GetIDGenerator().Generate()
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
	sim.MsgMeta
	WorkGroup  *kernels.WorkGroup
	PID        vm.PID
	Wavefronts []WfDispatchLocation
}

// Meta returns the meta data associated with the MapWGReq.
func (r *MapWGReq) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// MapWGReqBuilder can build MapWGReqs.
type MapWGReqBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
	pid      vm.PID
	wg       *kernels.WorkGroup
	wfs      []WfDispatchLocation
}

// WithSendTime sets the send time.
func (b MapWGReqBuilder) WithSendTime(t sim.VTimeInSec) MapWGReqBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the message.
func (b MapWGReqBuilder) WithSrc(src sim.Port) MapWGReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the message.
func (b MapWGReqBuilder) WithDst(dst sim.Port) MapWGReqBuilder {
	b.dst = dst
	return b
}

// WithWG sets the work-group to dispatch.
func (b MapWGReqBuilder) WithWG(wg *kernels.WorkGroup) MapWGReqBuilder {
	b.wg = wg
	return b
}

// WithPID sets the PID of the work-group.
func (b MapWGReqBuilder) WithPID(pid vm.PID) MapWGReqBuilder {
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
	r.Meta().ID = sim.GetIDGenerator().Generate()
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
	sim.MsgMeta
	RspTo []string
}

// Meta returns the meta data associated with the MapWGReq.
func (r *WGCompletionMsg) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// WGCompletionMsgBuilder can build MapWGReqs.
type WGCompletionMsgBuilder struct {
	sendTime sim.VTimeInSec
	src, dst sim.Port
	rspTo    []string
}

// WithSendTime sets the send time.
func (b WGCompletionMsgBuilder) WithSendTime(
	t sim.VTimeInSec,
) WGCompletionMsgBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the message.
func (b WGCompletionMsgBuilder) WithSrc(
	src sim.Port,
) WGCompletionMsgBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the message.
func (b WGCompletionMsgBuilder) WithDst(
	dst sim.Port,
) WGCompletionMsgBuilder {
	b.dst = dst
	return b
}

// WithRspTo sets rspTo
func (b WGCompletionMsgBuilder) WithRspTo(
	rspTo []string,
) WGCompletionMsgBuilder {
	b.rspTo = rspTo
	return b
}

// Build builds WGCompletionMsg
func (b WGCompletionMsgBuilder) Build() *WGCompletionMsg {
	msg := &WGCompletionMsg{}
	msg.Meta().ID = sim.GetIDGenerator().Generate()
	msg.Meta().SendTime = b.sendTime
	msg.Meta().Src = b.src
	msg.Meta().Dst = b.dst
	msg.RspTo = b.rspTo
	return msg
}
