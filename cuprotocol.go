package gcn3

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/kernels"
	"gitlab.com/akita/util/ca"
)

type CUPipelineDrainReq struct {
	akita.MsgMeta
	drainPipeline bool
}

// Meta returns the meta data associated with the message.
func (m *CUPipelineDrainReq) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

type CUPipelineDrainRsp struct {
	akita.MsgMeta
	drainPipelineComplete bool
}

// Meta returns the meta data associated with the message.
func (m *CUPipelineDrainRsp) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

type CUPipelineRestart struct {
	akita.MsgMeta
	restartPipeline bool
}

// Meta returns the meta data associated with the message.
func (m *CUPipelineRestart) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

type CUPipelineFlushReq struct {
	akita.MsgMeta
	flushPipeline bool
}

// Meta returns the meta data associated with the message.
func (m *CUPipelineFlushReq) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

type CUPipelineFlushRsp struct {
	akita.MsgMeta
	flushPipelineComplete bool
}

// Meta returns the meta data associated with the message.
func (m *CUPipelineFlushRsp) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

func NewCUPipelineDrainReq(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *CUPipelineDrainReq {
	req := new(CUPipelineDrainReq)

	req.SendTime = time
	req.Src = src
	req.Dst = dst

	req.drainPipeline = true

	return req
}

func NewCUPipelineDrainRsp(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *CUPipelineDrainRsp {
	req := new(CUPipelineDrainRsp)

	req.SendTime = time
	req.Src = src
	req.Dst = dst

	req.drainPipelineComplete = true

	return req
}

func NewCUPipelineRestartReq(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *CUPipelineRestart {
	req := new(CUPipelineRestart)

	req.SendTime = time
	req.Src = src
	req.Dst = dst

	req.restartPipeline = true

	return req
}

func NewCUPipelineFlushReq(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *CUPipelineFlushReq {
	req := new(CUPipelineFlushReq)

	req.SendTime = time
	req.Src = src
	req.Dst = dst

	req.flushPipeline = true

	return req
}

func NewCUPipelineFlushRsp(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *CUPipelineFlushRsp {
	req := new(CUPipelineFlushRsp)

	req.SendTime = time
	req.Src = src
	req.Dst = dst

	req.flushPipelineComplete = true

	return req
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
	r.Src = src
	r.Dst = dst
	r.SendTime = time
	r.WG = wg
	return r
}

// A WGFinishMesg is sent by a compute unit to notify about the completion of
// a work-group
type WGFinishMesg struct {
	akita.MsgMeta

	WG *kernels.WorkGroup
}

// Meta returns the meta data associated with the message.
func (m *WGFinishMesg) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewWGFinishMesg creates and returns a newly created WGFinishMesg
func NewWGFinishMesg(
	src, dst akita.Port,
	time akita.VTimeInSec,
	wg *kernels.WorkGroup,
) *WGFinishMesg {
	m := new(WGFinishMesg)

	m.Src = src
	m.Dst = dst
	m.SendTime = time
	m.WG = wg

	return m
}
