package timing

import "gitlab.com/yaotsu/core"

// An IssueInstReq is used to move one instruction from one unit to another
type IssueInstReq struct {
	*core.ReqBase
	Scheduler core.Component
	Wf        *Wavefront
}

// NewIssueInstReq creates a IssueInstReq that is to send from src to dst at
// time t. The wf has the instruction that is being moved.
func NewIssueInstReq(src, dst core.Component, t core.VTimeInSec,
	scheduler core.Component, wf *Wavefront,
) *IssueInstReq {
	req := new(IssueInstReq)
	req.ReqBase = core.NewReqBase()
	req.SetSrc(src)
	req.SetDst(dst)
	req.SetSendTime(t)
	req.Scheduler = scheduler
	req.Wf = wf
	return req
}
