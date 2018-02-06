package timing

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/mem"
)

// A Scheduler is the controlling unit of a compute unit. It decides which
// wavefront to fetch and to issue.
type Scheduler struct {
	cu           *ComputeUnit
	fetchArbiter WfArbiter
	issueArbiter WfArbiter
}

// NewScheduler returns a newly created scheduler, injecting dependency
// of the compute unit, the fetch arbiter, and the issue arbiter.
func NewScheduler(
	cu *ComputeUnit,
	fetchArbiter WfArbiter,
	issueArbiter WfArbiter,
) *Scheduler {
	s := new(Scheduler)
	s.cu = cu
	s.fetchArbiter = fetchArbiter
	s.issueArbiter = issueArbiter
	return s
}

// DoFetch function of the scheduler will fetch instructions from the
// instruction memory
func (s *Scheduler) DoFetch(now core.VTimeInSec) {
	wfs := s.fetchArbiter.Arbitrate(s.cu.WfPools)

	if len(wfs) > 0 {
		wf := wfs[0]
		wf.Inst = NewInst(nil)

		req := mem.NewAccessReq()
		req.Address = wf.PC
		req.Type = mem.Read
		req.ByteSize = 8
		req.SetDst(s.cu.InstMem)
		req.SetSrc(s.cu)
		req.SetSendTime(now)
		req.Info = wf

		s.cu.GetConnection("ToInstMem").Send(req)
		wf.State = WfFetching

		s.cu.InvokeHook(wf, s.cu, core.Any, &InstHookInfo{now, "FetchStart"})
	}
}

// DoIssue function of the scheduler issues fetched instruction to the decoding
// units
func (s *Scheduler) DoIssue(now core.VTimeInSec) {

}
