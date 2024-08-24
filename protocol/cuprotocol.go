package protocol

import (
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/kernels"
)

// A CUPipelineRestartReq is a message from CP to ask the CU pipeline to resume after a flush/drain
type CUPipelineRestartReq struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (m *CUPipelineRestartReq) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// Clone returns a clone of the CUPipelineRestartReq with different ID.
func (m *CUPipelineRestartReq) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// CUPipelineRestartReqBuilder can build new CU restart reqs
type CUPipelineRestartReqBuilder struct {
	src, dst sim.Port
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

// Clone returns a clone of the CUPipelineRestartRsp with different ID.
func (m *CUPipelineRestartRsp) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// CUPipelineRestartRspBuilder can build new CU restart reqs
type CUPipelineRestartRspBuilder struct {
	src, dst sim.Port
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

// Clone returns a clone of the CUPipelineFlushReq with different ID.
func (m *CUPipelineFlushReq) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// CUPipelineFlushReqBuilder can build new CU flush reqs
type CUPipelineFlushReqBuilder struct {
	src, dst sim.Port
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

// Clone returns a clone of the CUPipelineFlushRsp with different ID.
func (m *CUPipelineFlushRsp) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// CUPipelineFlushRspBuilder can build new CU flush rsps
type CUPipelineFlushRspBuilder struct {
	src, dst sim.Port
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

// Clone returns a clone of the MapWGReq with different ID.
func (r *MapWGReq) Clone() sim.Msg {
	cloneMsg := *r
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// MapWGReqBuilder can build MapWGReqs.
type MapWGReqBuilder struct {
	src, dst sim.Port
	pid      vm.PID
	wg       *kernels.WorkGroup
	wfs      []WfDispatchLocation
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

// Clone returns a clone of the WGCompletionMsg with different ID.
func (r *WGCompletionMsg) Clone() sim.Msg {
	cloneMsg := *r
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// WGCompletionMsgBuilder can build MapWGReqs.
type WGCompletionMsgBuilder struct {
	src, dst sim.Port
	rspTo    []string
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
	msg.Meta().Src = b.src
	msg.Meta().Dst = b.dst
	msg.RspTo = b.rspTo
	return msg
}
