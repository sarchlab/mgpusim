package timing

import (
	"log"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)

// A Scheduler is the controlling unit of a compute unit. It decides which
// wavefront to fetch and to issue.
type Scheduler struct {
	cu                *ComputeUnit
	fetchArbiter      WfArbiter
	issueArbiter      WfArbiter
	internalExecuting *Wavefront
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
		wf.inst = NewInst(nil)
		// log.Printf("fetching wf %d pc %d\n", wf.FirstWiFlatID, wf.PC)

		s.cu.InvokeHook(wf, s.cu, core.Any, &InstHookInfo{now, "FetchStart"})

		req := mem.NewAccessReq()
		req.Address = wf.PC
		req.Type = mem.Read
		req.ByteSize = 8
		req.SetDst(s.cu.InstMem)
		req.SetSrc(s.cu)
		req.SetSendTime(now)
		info := new(MemAccessInfo)
		info.Action = MemAccessInstFetch
		info.Wf = wf
		req.Info = info

		s.cu.GetConnection("ToInstMem").Send(req)
		wf.State = WfFetching
	}
}

// DoIssue function of the scheduler issues fetched instruction to the decoding
// units
func (s *Scheduler) DoIssue(now core.VTimeInSec) {
	wfs := s.issueArbiter.Arbitrate(s.cu.WfPools)
	for _, wf := range wfs {

		if wf.Inst().ExeUnit == insts.ExeUnitSpecial {
			s.issueToInternal(wf, now)
			continue
		}

		unit := s.getUnitToIssueTo(wf.Inst().ExeUnit)
		if unit.CanAcceptWave() {
			unit.AcceptWave(wf, now)
			// log.Printf("%f: %s from wf %d issued.\n", now, wf.Inst.String(), wf.FirstWiFlatID)
			wf.State = WfRunning
			s.cu.InvokeHook(wf, s.cu, core.Any, &InstHookInfo{now, "Issue"})
		}
	}
}

func (s *Scheduler) issueToInternal(wf *Wavefront, now core.VTimeInSec) {
	if s.internalExecuting == nil {
		s.internalExecuting = wf
		wf.State = WfRunning
		//wf.PC += uint64(wf.Inst().ByteSize)
	} else {
		wf.State = WfFetched
	}

}

func (s *Scheduler) getUnitToIssueTo(u insts.ExeUnit) CUComponent {
	switch u {
	case insts.ExeUnitBranch:
		return s.cu.BranchUnit
	case insts.ExeUnitLDS:
		return s.cu.LDSDecoder
	case insts.ExeUnitVALU:
		return s.cu.VectorDecoder
	case insts.ExeUnitVMem:
		return s.cu.VectorMemDecoder
	case insts.ExeUnitScalar:
		return s.cu.ScalarDecoder
	default:
		log.Panic("not sure where to dispatch instrcution")
	}
	return nil
}

// EvaluateInternalInst updates the status of the instruction being executed
// in the scheduler.
func (s *Scheduler) EvaluateInternalInst(now core.VTimeInSec) {
	if s.internalExecuting == nil {
		return
	}

	executing := s.internalExecuting

	switch s.internalExecuting.Inst().Opcode {
	case 1: // S_ENDPGM
		s.evalSEndPgm(s.internalExecuting, now)
	case 12: // S_WAITCNT
		s.evalSWaitCnt(s.internalExecuting, now)
	default:
		// The program has to make progress
		s.internalExecuting.State = WfReady
		s.internalExecuting = nil
	}

	if s.internalExecuting == nil {
		s.cu.InvokeHook(executing, s.cu, core.Any,
			&InstHookInfo{now, "Completed"})
	}
}

func (s *Scheduler) evalSEndPgm(wf *Wavefront, now core.VTimeInSec) {
	wfCompletionEvt := NewWfCompletionEvent(s.cu.Freq.NextTick(now), s.cu, wf)
	s.cu.engine.Schedule(wfCompletionEvt)
	s.internalExecuting = nil
}

func (s *Scheduler) evalSWaitCnt(wf *Wavefront, now core.VTimeInSec) {
	done := true
	inst := wf.Inst()

	if wf.OutstandingScalarMemAccess > inst.LKGMCNT {
		done = false
	}

	if done {
		s.internalExecuting.State = WfReady
		s.internalExecuting = nil
	}
}
