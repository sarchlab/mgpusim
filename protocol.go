package gcn3

import "gitlab.com/akita/akita"

type CUPipelineDrainReq struct {
	*akita.ReqBase
	drainPipeline bool
}

type CUPipelineDrainRsp struct {
	*akita.ReqBase
	drainPipelineComplete bool
}

type CUPipelineRestart struct {
	*akita.ReqBase
	restartPipeline bool
}

type CUPipelineFlush struct {
	*akita.ReqBase
	flushPipeline bool
}

type CUPipelineFlushRsp struct {
	*akita.ReqBase
	flushPipelineComplete bool
}

func NewCUPipelineDrainReq(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *CUPipelineDrainReq {
	reqBase := akita.NewReqBase()
	req := new(CUPipelineDrainReq)
	req.ReqBase = reqBase

	req.SetSendTime(time)
	req.SetSrc(src)
	req.SetDst(dst)

	req.drainPipeline = true

	return req
}

func NewCUPipelineDrainRsp(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *CUPipelineDrainRsp {
	reqBase := akita.NewReqBase()
	req := new(CUPipelineDrainRsp)
	req.ReqBase = reqBase

	req.SetSendTime(time)
	req.SetSrc(src)
	req.SetDst(dst)

	req.drainPipelineComplete = true

	return req
}

func NewCUPipelineRestartReq(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *CUPipelineRestart {
	reqBase := akita.NewReqBase()
	req := new(CUPipelineRestart)
	req.ReqBase = reqBase

	req.SetSendTime(time)
	req.SetSrc(src)
	req.SetDst(dst)

	req.restartPipeline = true

	return req
}

func NewCUPipelineFlush(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *CUPipelineFlush {
	reqBase := akita.NewReqBase()
	req := new(CUPipelineFlush)
	req.ReqBase = reqBase

	req.SetSendTime(time)
	req.SetSrc(src)
	req.SetDst(dst)

	req.flushPipeline = true

	return req
}

func NewCUPipelineFlushRsp(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *CUPipelineFlushRsp {
	reqBase := akita.NewReqBase()
	req := new(CUPipelineFlushRsp)
	req.ReqBase = reqBase

	req.SetSendTime(time)
	req.SetSrc(src)
	req.SetDst(dst)

	req.flushPipelineComplete = true

	return req
}
