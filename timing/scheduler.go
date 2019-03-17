package timing

import (
	"log"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/timing/wavefront"
	"gitlab.com/akita/mem"
)

type Scheduler interface {
	Run(now akita.VTimeInSec) bool
	Pause()
	Resume()
	Flush()
}

// A Scheduler is the controlling unit of a compute unit. It decides which
// wavefront to fetch and to issue.
type SchedulerImpl struct {
	cu                *ComputeUnit
	fetchArbiter      WfArbiter
	issueArbiter      WfArbiter
	internalExecuting *wavefront.Wavefront

	barrierBuffer     []*wavefront.Wavefront
	barrierBufferSize int

	cyclesNoProgress                  int
	stopTickingAfterNCyclesNoProgress int

	isPaused bool
}

// NewScheduler returns a newly created scheduler, injecting dependency
// of the compute unit, the fetch arbiter, and the issue arbiter.
func NewScheduler(
	cu *ComputeUnit,
	fetchArbiter WfArbiter,
	issueArbiter WfArbiter,
) *SchedulerImpl {
	s := new(SchedulerImpl)
	s.cu = cu
	s.fetchArbiter = fetchArbiter
	s.issueArbiter = issueArbiter

	s.barrierBufferSize = 16
	s.barrierBuffer = make([]*wavefront.Wavefront, 0, s.barrierBufferSize)

	s.stopTickingAfterNCyclesNoProgress = 4

	return s
}

func (s *SchedulerImpl) Run(now akita.VTimeInSec) bool {
	madeProgress := false
	if s.isPaused == false {
		madeProgress = s.EvaluateInternalInst(now) || madeProgress
		madeProgress = s.DecodeNextInst(now) || madeProgress
		madeProgress = s.DoIssue(now) || madeProgress
		madeProgress = s.DoFetch(now) || madeProgress
	}
	if !madeProgress {
		s.cyclesNoProgress++
	} else {
		s.cyclesNoProgress = 0
	}

	if s.cyclesNoProgress > s.stopTickingAfterNCyclesNoProgress {
		return false
	}
	return true
}

func (s *SchedulerImpl) DecodeNextInst(now akita.VTimeInSec) bool {
	madeProgress := false
	for _, wfPool := range s.cu.WfPools {
		for _, wf := range wfPool.wfs {
			if len(wf.InstBuffer) == 0 {
				wf.InstBufferStartPC = wf.PC & 0xffffffffffffffc0
				continue
			}

			if wf.State != wavefront.WfReady {
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
				wf.InstToIssue = wavefront.NewInst(inst)
				s.cu.InvokeHook(wf, s.cu, akita.AnyHookPos,
					&wavefront.InstHookInfo{
						now, wf.InstToIssue, "Create"})
				madeProgress = true
			}
		}
	}
	return madeProgress
}

//func (s *Scheduler) wfHasAtLeast8BytesInInstBuffer(wf *wavefront.Wavefront) bool {
//	return wf.InstBufferStartPC+uint64(len(wf.InstBuffer)) >= wf.PC+8
//}

// DoFetch function of the scheduler will fetch instructions from the
// instruction memory
func (s *SchedulerImpl) DoFetch(now akita.VTimeInSec) bool {
	madeProgress := false
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
		req.IsPhysical = false
		req.PID = wf.PID()

		err := s.cu.ToInstMem.Send(req)
		if err == nil {
			info := new(InstFetchReqInfo)
			info.Wavefront = wf
			info.Req = req
			info.Address = addr
			s.cu.InFlightInstFetch = append(s.cu.InFlightInstFetch, info)
			wf.IsFetching = true

			//s.cu.InvokeHook(wf, s.cu, akita.AnyHookPos, &InstHookInfo{now, wf.inst, "FetchStart"})
			madeProgress = true
		}
	}

	return madeProgress
}

// DoIssue function of the scheduler issues fetched instruction to the decoding
// units
func (s *SchedulerImpl) DoIssue(now akita.VTimeInSec) bool {
	madeProgress := false

	if s.isPaused == false {
		wfs := s.issueArbiter.Arbitrate(s.cu.WfPools)
		for _, wf := range wfs {
			if wf.InstToIssue.ExeUnit == insts.ExeUnitSpecial {
				madeProgress = s.issueToInternal(wf, now) || madeProgress

				continue
			}

			unit := s.getUnitToIssueTo(wf.InstToIssue.ExeUnit)
			if unit.CanAcceptWave() {
				wf.SetDynamicInst(wf.InstToIssue)
				wf.InstToIssue = nil
				unit.AcceptWave(wf, now)
				wf.State = wavefront.WfRunning
				//s.removeStaleInstBuffer(wf)
				s.cu.InvokeHook(wf, s.cu, akita.AnyHookPos,
					&wavefront.InstHookInfo{now, wf.InstToIssue, "Issue"})
				madeProgress = true
			}
		}
	}
	return madeProgress
}

func (s *SchedulerImpl) issueToInternal(wf *wavefront.Wavefront, now akita.VTimeInSec) bool {
	if s.internalExecuting == nil {
		wf.SetDynamicInst(wf.InstToIssue)
		wf.InstToIssue = nil
		s.internalExecuting = wf
		wf.State = wavefront.WfRunning
		//s.removeStaleInstBuffer(wf)
		s.cu.InvokeHook(wf, s.cu, akita.AnyHookPos, &wavefront.InstHookInfo{now, wf.InstToIssue, "Issue"})
		return true
	}
	return false
}

func (s *SchedulerImpl) getUnitToIssueTo(u insts.ExeUnit) CUComponent {
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
		log.Panic("not sure where to dispatch the instruction")
	}
	return nil
}

// EvaluateInternalInst updates the status of the instruction being executed
// in the scheduler.
func (s *SchedulerImpl) EvaluateInternalInst(now akita.VTimeInSec) bool {
	if s.internalExecuting == nil {
		return false
	}

	madeProgress := false
	executing := s.internalExecuting

	switch s.internalExecuting.Inst().Opcode {
	case 1: // S_ENDPGM
		madeProgress = s.evalSEndPgm(s.internalExecuting, now) || madeProgress
	case 10: // S_BARRIER
		madeProgress = s.evalSBarrier(s.internalExecuting, now) || madeProgress
	case 12: // S_WAITCNT
		madeProgress = s.evalSWaitCnt(s.internalExecuting, now) || madeProgress
	default:
		// The program has to make progress
		s.internalExecuting.State = wavefront.WfReady
		s.internalExecuting = nil
		madeProgress = true
	}

	if s.internalExecuting == nil {
		s.cu.InvokeHook(executing, s.cu, akita.AnyHookPos,
			&wavefront.InstHookInfo{now, executing.DynamicInst(), "Completed"})
	}
	return madeProgress
}

func (s *SchedulerImpl) evalSEndPgm(wf *wavefront.Wavefront, now akita.VTimeInSec) bool {
	if wf.OutstandingVectorMemAccess > 0 || wf.OutstandingScalarMemAccess > 0 {
		return false
	}
	wfCompletionEvt := NewWfCompletionEvent(s.cu.Freq.NextTick(now), s.cu, wf)
	s.cu.Engine.Schedule(wfCompletionEvt)
	s.internalExecuting = nil

	s.resetRegisterValue(wf)
	return true
}

func (s *SchedulerImpl) resetRegisterValue(wf *wavefront.Wavefront) {
	if wf.CodeObject.WIVgprCount > 0 {
		vRegFile := s.cu.VRegFile[wf.SIMDID].(*SimpleRegisterFile)
		vRegStorage := vRegFile.storage
		data := make([]byte, wf.CodeObject.WIVgprCount*4)
		for i := 0; i < 64; i++ {
			offset := uint64(wf.VRegOffset + vRegFile.ByteSizePerLane*i)
			copy(vRegStorage[offset:], data)
		}
	}

	if wf.CodeObject.WFSgprCount > 0 {
		sRegFile := s.cu.SRegFile.(*SimpleRegisterFile)
		sRegStorage := sRegFile.storage
		data := make([]byte, wf.CodeObject.WFSgprCount*4)
		offset := uint64(wf.SRegOffset)
		copy(sRegStorage[offset:], data)
	}
}

func (s *SchedulerImpl) evalSBarrier(wf *wavefront.Wavefront, now akita.VTimeInSec) bool {
	wf.State = wavefront.WfAtBarrier

	wg := wf.WG
	allAtBarrier := s.areAllWfInWGAtBarrier(wg)

	if allAtBarrier {
		s.passBarrier(wg)
		s.internalExecuting = nil
		return true
	} else {
		if len(s.barrierBuffer) < s.barrierBufferSize {
			s.barrierBuffer = append(s.barrierBuffer, wf)
			s.internalExecuting = nil
			return true
		}
		return false
	}

	return true
}

func (s *SchedulerImpl) areAllWfInWGAtBarrier(wg *wavefront.WorkGroup) bool {
	for _, wf := range wg.Wfs {
		if wf.State != wavefront.WfAtBarrier {
			return false
		}
	}
	return true
}

func (s *SchedulerImpl) passBarrier(wg *wavefront.WorkGroup) {
	s.removeAllWfFromBarrierBuffer(wg)
	s.setAllWfStateToReady(wg)
}

func (s *SchedulerImpl) setAllWfStateToReady(wg *wavefront.WorkGroup) {
	for _, wf := range wg.Wfs {
		s.cu.UpdatePCAndSetReady(wf)
	}
}

func (s *SchedulerImpl) removeAllWfFromBarrierBuffer(wg *wavefront.WorkGroup) {
	newBarrierBuffer := make([]*wavefront.Wavefront, 0, s.barrierBufferSize)
	for _, wavefront := range s.barrierBuffer {
		if wavefront.WG != wg {
			newBarrierBuffer = append(newBarrierBuffer, wavefront)
		}
	}
	s.barrierBuffer = newBarrierBuffer
}

func (s *SchedulerImpl) evalSWaitCnt(wf *wavefront.Wavefront, now akita.VTimeInSec) bool {
	done := true
	inst := wf.Inst()

	if wf.OutstandingScalarMemAccess > inst.LKGMCNT {
		done = false
	}

	if wf.OutstandingVectorMemAccess > inst.VMCNT {
		done = false
	}

	if done {
		s.cu.UpdatePCAndSetReady(s.internalExecuting)
		s.internalExecuting = nil
		return true
	}
	return false
}

func (s *SchedulerImpl) Pause() {
	s.isPaused = true
}

func (s *SchedulerImpl) Resume() {
	s.isPaused = false
}

func (s *SchedulerImpl) Flush() {
	s.barrierBuffer = nil
	s.internalExecuting = nil
}
