package cu

import "gitlab.com/yaotsu/core"

// IssueReq is a request that is sent from the scheduler to the instruction
// units to issue instructions
type IssueReq struct {
	*core.BasicRequest

	Wf   *Wavefront
	Inst *Inst
}
