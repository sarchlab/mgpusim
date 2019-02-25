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
