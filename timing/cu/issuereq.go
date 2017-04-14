package cu

import "gitlab.com/yaotsu/core"

// IssueReq is a request that is sent from the scheduler to the instruction
// units to issue instructions
type IssueReq struct {
	*core.ReqBase

	Wf   *Wavefront
	Inst *Inst
}
