package cu

import "gitlab.com/yaotsu/core"

// InstCompletionReq is the request sent by the execution unit to the scheduler
// to notify the completion of an instruction. The scheduler can then schedule
// another instruction from the wavefront
type InstCompletionReq struct {
	*core.ReqBase

	Wf *Wavefront
}

// NewInstCompletionReq creates a InstCompletionReq
func NewInstCompletionReq(
	src, dst core.Component,
	t core.VTimeInSec,
	wf *Wavefront,
) *InstCompletionReq {
	r := new(InstCompletionReq)
	r.ReqBase = core.NewReqBase()
	r.SetSrc(src)
	r.SetDst(dst)
	r.SetSendTime(t)
	r.Wf = wf
	return r
}
