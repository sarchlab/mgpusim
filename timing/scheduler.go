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

	barrierBuffer     []*Wavefront
	barrierBufferSize int
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

	s.barrierBufferSize = 16
	s.barrierBuffer = make([]*Wavefront, 0, s.barrierBufferSize)

	return s
}

func (s *Scheduler) Run(now core.VTimeInSec) {
	s.EvaluateInternalInst(now)
	s.DecodeNextInst()
	s.DoIssue(now)
	s.DoFetch(now)
}

func (s *Scheduler) DecodeNextInst() {
	for _, wfPool := range s.cu.WfPools {
		for _, wf := range wfPool.wfs {
			if len(wf.InstBuffer) == 0 {
				wf.InstBufferStartPC = wf.PC & 0xffffffffffffffc0
				continue
			}

			if wf.State != WfReady {
				continue
			}

			if wf.InstToIssue != nil {
				continue
			}
			//
			//if !s.wfHasAtLeast8BytesInInstBuffer(wf) {
			//	continue
			//}

			inst, err := s.cu.Decoder.Decode(
				wf.InstBuffer[wf.PC-wf.InstBufferStartPC:])
			if err == nil {
				wf.InstToIssue = NewInst(inst)
			}
		}
	}
}

//func (s *Scheduler) wfHasAtLeast8BytesInInstBuffer(wf *Wavefront) bool {
//	return wf.InstBufferStartPC+uint64(len(wf.InstBuffer)) >= wf.PC+8
//}

// DoFetch function of the scheduler will fetch instructions from the
// instruction memory
func (s *Scheduler) DoFetch(now core.VTimeInSec) {
	wfs := s.fetchArbiter.Arbitrate(s.cu.WfPools)

	if len(wfs) > 0 {
		wf := wfs[0]
		//wf.inst = NewInst(nil)
		// log.Printf("fetching wf %d pc %d\n", wf.FirstWiFlatID, wf.PC)

		if len(wf.InstBuffer) == 0 {
			wf.InstBufferStartPC = wf.PC & 0xffffffffffffffc0
		}
		addr := wf.InstBufferStartPC + uint64(len(wf.InstBuffer))
		addr = addr & 0xffffffffffffffc0
		req := mem.NewReadReq(now, s.cu.ToInstMem, s.cu.InstMem, addr, 64)

		err := s.cu.ToInstMem.Send(req)
		if err == nil {
			info := new(MemAccessInfo)
			info.Action = MemAccessInstFetch
			info.Wf = wf
			info.Address = addr
			s.cu.inFlightMemAccess[req.ID] = info
			wf.IsFetching = true

			//s.cu.InvokeHook(wf, s.cu, core.Any, &InstHookInfo{now, wf.inst, "FetchStart"})
		}
	}
}

// DoIssue function of the scheduler issues fetched instruction to the decoding
// units
func (s *Scheduler) DoIssue(now core.VTimeInSec) {
	wfs := s.issueArbiter.Arbitrate(s.cu.WfPools)
	for _, wf := range wfs {
		if wf.InstToIssue.ExeUnit == insts.ExeUnitSpecial {
			s.issueToInternal(wf, now)
			continue
		}

		unit := s.getUnitToIssueTo(wf.InstToIssue.ExeUnit)
		if unit.CanAcceptWave() {
			wf.inst = wf.InstToIssue
			wf.InstToIssue = nil
			unit.AcceptWave(wf, now)
			wf.State = WfRunning
			wf.PC += uint64(wf.inst.ByteSize)
			s.removeStaleInstBuffer(wf)
			s.cu.InvokeHook(wf, s.cu, core.Any, &InstHookInfo{now, wf.inst, "Issue"})
		}
	}
}

func (s *Scheduler) removeStaleInstBuffer(wf *Wavefront) {
	for wf.PC >= wf.InstBufferStartPC+64 {
		wf.InstBuffer = wf.InstBuffer[64:]
		wf.InstBufferStartPC += 64
	}
}

func (s *Scheduler) issueToInternal(wf *Wavefront, now core.VTimeInSec) {
	if s.internalExecuting == nil {
		wf.inst = wf.InstToIssue
		wf.InstToIssue = nil
		s.internalExecuting = wf
		wf.State = WfRunning
		wf.PC += uint64(wf.Inst().ByteSize)
		s.removeStaleInstBuffer(wf)
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
	case 10: // S_BARRIER
		s.evalSBarrier(s.internalExecuting, now)
	case 12: // S_WAITCNT
		s.evalSWaitCnt(s.internalExecuting, now)
	default:
		// The program has to make progress
		s.internalExecuting.State = WfReady
		s.internalExecuting = nil
	}

	if s.internalExecuting == nil {
		s.cu.InvokeHook(executing, s.cu, core.Any,
			&InstHookInfo{now, executing.inst, "Completed"})
	}
}

func (s *Scheduler) evalSEndPgm(wf *Wavefront, now core.VTimeInSec) {
	if wf.OutstandingVectorMemAccess > 0 || wf.OutstandingScalarMemAccess > 0 {
		return
	}
	wfCompletionEvt := NewWfCompletionEvent(s.cu.Freq.NextTick(now), s.cu, wf)
	s.cu.engine.Schedule(wfCompletionEvt)
	s.internalExecuting = nil
}

func (s *Scheduler) evalSBarrier(wf *Wavefront, now core.VTimeInSec) {
	wg := wf.WG
	allAtBarrier := true
	for _, wavefront := range wg.Wfs {
		if wavefront == wf {
			continue
		}

		if wavefront.State != WfAtBarrier {
			allAtBarrier = false
			break
		}
	}

	if allAtBarrier {
		for _, wavefront := range wg.Wfs {
			wavefront.State = WfReady
		}
		s.internalExecuting = nil
	}

	newBarrierBuffer := make([]*Wavefront, 0, s.barrierBufferSize)
	for _, wavefront := range s.barrierBuffer {
		if wavefront.State == WfAtBarrier {
			newBarrierBuffer = append(newBarrierBuffer, wavefront)
		}
	}
	s.barrierBuffer = newBarrierBuffer

	if !allAtBarrier {
		if len(s.barrierBuffer) < s.barrierBufferSize {
			wf.State = WfAtBarrier
			s.barrierBuffer = append(s.barrierBuffer, wf)
			s.internalExecuting = nil
		}
	}
}

func (s *Scheduler) evalSWaitCnt(wf *Wavefront, now core.VTimeInSec) {
	done := true
	inst := wf.Inst()

	if wf.OutstandingScalarMemAccess > inst.LKGMCNT {
		done = false
	}

	if wf.OutstandingVectorMemAccess > inst.VMCNT {
		done = false
	}

	if done {
		s.internalExecuting.State = WfReady
		s.internalExecuting = nil
	}
}
