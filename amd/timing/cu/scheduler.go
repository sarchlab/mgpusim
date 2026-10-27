package cu

import (
	"log"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// Scheduler does its job
type Scheduler interface {
	Run() bool
}

// SchedulerImpl implements scheduler
// A Scheduler is the controlling unit of a compute unit. It decides which
// wavefront to fetch and to issue.
type SchedulerImpl struct {
	cu           *ComputeUnit
	fetchArbiter WfArbiter
	issueArbiter WfArbiter

	barrierBufferSize int

	stopTickingAfterNCyclesNoProgress int

	scoreboardEnabled bool
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

	s.stopTickingAfterNCyclesNoProgress = 4

	return s
}

// Run runs scheduler
func (s *SchedulerImpl) Run() bool {
	madeProgress := false
	if s.scoreboardEnabled {
		madeProgress = s.tickScoreboards() || madeProgress
	}
	madeProgress = s.EvaluateInternalInst() || madeProgress
	madeProgress = s.DecodeNextInst() || madeProgress
	madeProgress = s.DoIssue() || madeProgress
	madeProgress = s.DoFetch() || madeProgress
	if !madeProgress {
		s.state().NoProgress++
	} else {
		s.state().NoProgress = 0
	}

	if s.state().NoProgress > s.stopTickingAfterNCyclesNoProgress {
		return false
	}
	return true
}

func (s *SchedulerImpl) tickScoreboards() bool {
	ticked := false
	for _, wfPool := range s.cu.WfPools {
		for _, handle := range wfPool.ids() {
			wf := wfPool.cu.wave(handle)
			if wf.scoreboard() != nil {
				sb := wf.scoreboard()
				if sb.AnyBusy() {
					sb.Tick()
					ticked = true
				}
			}
		}
	}
	return ticked
}

// DecodeNextInst checks
func (s *SchedulerImpl) DecodeNextInst() bool {
	madeProgress := false
	for _, wfPool := range s.cu.WfPools {
		for _, handle := range wfPool.ids() {
			wf := wfPool.cu.wave(handle)
			if len(wf.execution().InstBuffer) == 0 {
				wf.execution().InstBufferStartPC = wf.PC() & 0xffffffffffffffc0
				continue
			}

			if wf.execution().State != wavefront.WfReady {
				continue
			}

			if wf.execution().ToIssueID != 0 {
				continue
			}

			if !s.wfHasAtLeast4BytesInInstBuffer(wf) {
				continue
			}

			inst, err := s.cu.Decoder.Decode(
				wf.execution().InstBuffer[wf.PC()-wf.execution().InstBufferStartPC:])
			if err == nil {
				next := wavefront.NewInst(s.cu.comp.NewID(), inst)
				start := wf.PC() - wf.execution().InstBufferStartPC
				next.Raw = append([]byte(nil), wf.execution().InstBuffer[start:start+uint64(inst.ByteSize)]...)
				wf.SetNextInst(next)
				madeProgress = true
			}
		}
	}
	return madeProgress
}

func (s *SchedulerImpl) wfHasAtLeast4BytesInInstBuffer(
	wf *Wavefront,
) bool {
	return len(wf.execution().InstBuffer[wf.PC()-wf.execution().InstBufferStartPC:]) >= 4
}

// DoFetch function of the scheduler will fetch instructions from the
// instruction memory. It fetches for up to 4 wavefronts per cycle to
// reduce startup latency for streaming workloads.
func (s *SchedulerImpl) DoFetch() bool {
	madeProgress := false
	wfs := s.fetchArbiter.Arbitrate(s.cu.WfPools)

	fetchLimit := min(4, len(wfs))
	for idx := 0; idx < fetchLimit; idx++ {
		wf := wfs[idx]

		if len(wf.execution().InstBuffer) == 0 {
			wf.execution().InstBufferStartPC = wf.PC() & 0xffffffffffffffc0
		}
		addr := wf.execution().InstBufferStartPC + uint64(len(wf.execution().InstBuffer))
		addr = addr & 0xffffffffffffffc0
		req := messaging.Msg{ID: s.cu.comp.NewID(),
			Src: s.cu.instMemPort().AsRemote(),
			Dst: s.cu.comp.Spec.InstMem, Payload: memprotocol.ReadReq{Address: addr,
				AccessByteSize: 64,
				PID:            wf.PID()}}

		if !s.cu.instMemPort().CanSend() {
			continue
		}

		info := new(InstFetchReqInfo)
		info.Wavefront = wf
		info.Req = req
		info.Address = addr

		wf.execution().IsFetching = true

		madeProgress = true

		info.FetchTaskID = s.cu.comp.TaskTracker().StartTask(s.cu.comp, tasks.StartInfo{
			ParentID: s.cu.comp.State.WFTasks[wf.UID],
			Kind:     "fetch",
			What:     "fetch",
			// Give instruction fetch its own unit location instead of letting it
			// fall back to the bare CU name (singleKindLocation's default). That
			// keeps "one location, one kind": the bare CU name is no longer a task
			// row, and fetch can't collide with the sampled wavefront task (which
			// also used to fall back to the bare CU name).
			Location: s.cu.comp.Name() + ".InstFetcher",
		})
		if wf.execution().InFlightInsts == 0 && !wfHasInstructionReady(wf) {
			// Fetching while nothing is in flight: the wavefront is stalled with no
			// instruction available, waiting on the fetch path. (A prefetch issued
			// while an instruction executes leaves InFlightInsts > 0 and is silent;
			// a fetch-ahead with decoded/available bytes is also silent because the
			// real blocker is the execution unit that has not accepted the ready
			// instruction yet.
			s.cu.markWfMilestone(wf, tasks.MilestoneKindHardwareResource,
				s.cu.comp.Name()+".InstFetcher")
		}
		s.cu.comp.State.InstFetch = append(s.cu.comp.State.InstFetch, s.cu.fetchState(info))
		s.cu.startRequest(req, info.FetchTaskID)
		s.cu.instMemPort().Send(req)
	}

	return madeProgress
}

// DoIssue function of the scheduler issues fetched instruction to the decoding
// units
func (s *SchedulerImpl) DoIssue() bool {
	madeProgress := false

	wfs := s.issueArbiter.Arbitrate(s.cu.WfPools)
	for _, wf := range wfs {
		inst := wf.NextInst()
		if inst.ExeUnit == insts.ExeUnitSpecial {
			madeProgress = s.issueToInternal(wf) || madeProgress

			continue
		}

		unit := s.getUnitToIssueTo(inst.ExeUnit)
		if unit.CanAcceptWave() {
			wf.issueNext()

			s.cu.beginInstruction(wf, inst)

			if s.scoreboardEnabled && wf.scoreboard() != nil {
				latency := GetScoreboardLatency(inst.Inst)
				if latency > 0 {
					wf.scoreboard().MarkBusy(
						inst.Inst, latency)
				}
			}

			unit.AcceptWave(wf)
			wf.execution().State = wavefront.WfRunning

			madeProgress = true
		}
	}
	return madeProgress
}

func (s *SchedulerImpl) issueToInternal(wf *Wavefront) bool {
	wf.issueNext()
	s.state().Internal = append(s.state().Internal, s.cu.useWave(wf))
	wf.execution().State = wavefront.WfRunning

	s.cu.beginInstruction(wf, wf.DynamicInst())

	return true
}

func (s *SchedulerImpl) getUnitToIssueTo(u insts.ExeUnit) SubComponent {
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
func (s *SchedulerImpl) EvaluateInternalInst() bool {
	if s.state().Internal == nil {
		return false
	}

	madeProgress := false

	newExecuting := make([]uint64, 0)
	for _, id := range s.state().Internal {
		executing := s.cu.wave(id)
		instruction := executing.DynamicInst()
		instProgress := false
		instCompleted := false
		passBarrier := false

		switch instruction.Opcode {
		case 1: // S_ENDPGM
			instProgress, instCompleted = s.evalSEndPgm(executing)
		case 10: // S_BARRIER
			instProgress, instCompleted, passBarrier = s.evalSBarrier(executing)

			if passBarrier {
				s.removeAllWfFromInternalExecuting(executing.group(), &newExecuting)
				s.removeAllWfFromInternalExecuting(
					executing.group(), &s.state().Internal)
			}
		case 12: // S_WAITCNT
			instProgress, instCompleted = s.evalSWaitCnt(executing)
		default:
			// The program has to make progress
			s.cu.UpdatePCAndSetReady(executing)
			instProgress = true
			instCompleted = true
		}
		madeProgress = instProgress || madeProgress

		if instCompleted {
			s.cu.completeInstruction(executing, instruction)
			if executionEnded(executing.execution().State) {
				s.cu.endWaveTask(executing)
			}
		} else {
			newExecuting = append(newExecuting, id)
		}
	}

	s.state().Internal = newExecuting

	return madeProgress
}

func (s *SchedulerImpl) evalSEndPgm(
	wf *Wavefront,
) (madeProgress bool, instCompleted bool) {
	wf.state().StopFetch = true
	if wf.execution().OutstandingVectorMemAccess > 0 ||
		wf.execution().OutstandingScalarMemAccess > 0 {
		return false, false
	}

	// sampling
	now := s.cu.CurrentTime()
	if s.cu.comp.Resources.Sampler != nil && !s.cu.comp.Resources.Sampler.IfDisable() {
		issuetime, found := s.cu.comp.State.WFTimes[wf.UID]
		if found {
			finishtime := now
			wf.execution().SampleFinishTime = finishtime
			wf.execution().SampleIssueTime = issuetime
			delete(s.cu.comp.State.WFTimes, wf.UID)
		}
	}

	s.markMemDrained(wf, "s_endpgm")
	s.cu.markWfMilestone(wf, tasks.MilestoneKindWork, "execution")
	if s.areAllOtherWfsInWGAtBarrier(wf.group(), wf) && !s.areAllOtherWfsExecutionEnded(wf.group(), wf) {
		s.passBarrier(wf.group())
	}
	s.resetRegisterValue(wf)
	wf.execution().State = wavefront.WfCompleted
	if wf.execution().IsFetching {
		wf.execution().State = wavefront.WfDraining
	}

	return true, true
}

func (s *SchedulerImpl) areAllOtherWfsExecutionEnded(
	wg *WorkGroup,
	currWf *Wavefront,
) bool {
	for _, wf := range wg.waves() {
		if wf.UID == currWf.UID {
			continue
		}

		if !executionEnded(wf.execution().State) {
			return false
		}
	}

	return true
}

func (s *SchedulerImpl) areAllOtherWfsInWGAtBarrier(
	wg *WorkGroup,
	currWf *Wavefront,
) bool {
	for _, wf := range wg.waves() {
		if wf.UID == currWf.UID {
			continue
		}

		if wf.execution().State != wavefront.WfAtBarrier &&
			!executionEnded(wf.execution().State) {
			return false
		}
	}

	return true
}

func (s *SchedulerImpl) resetRegisterValue(wf *Wavefront) {
	if wf.codeObject().WIVgprCount > 0 {
		vRegFile := s.cu.VRegFile[wf.execution().SIMDID].(*SimpleRegisterFile)
		vRegStorage := vRegFile.bytes()
		data := make([]byte, wf.codeObject().WIVgprCount*4)
		for i := 0; i < 64; i++ {
			offset := uint64(wf.execution().VRegOffset + vRegFile.ByteSizePerLane*i)
			copy(vRegStorage[offset:], data)
		}
	}

	if wf.codeObject().WFSgprCount > 0 {
		sRegFile := s.cu.SRegFile.(*SimpleRegisterFile)
		sRegStorage := sRegFile.bytes()
		data := make([]byte, wf.codeObject().WFSgprCount*4)
		offset := uint64(wf.execution().SRegOffset)
		copy(sRegStorage[offset:], data)
	}
}

func (s *SchedulerImpl) evalSBarrier(
	wf *Wavefront,
) (madeProgress bool, instCompleted bool, passBarrier bool) {
	wf.execution().State = wavefront.WfAtBarrier

	wg := wf.group()
	allAtBarrier := s.areAllWfInWGAtBarrier(wg)

	if allAtBarrier {
		s.passBarrier(wg)
		return true, true, true
	}

	if len(s.state().Barrier) < s.barrierBufferSize {
		s.state().Barrier = append(s.state().Barrier, s.cu.useWave(wf))
		return true, true, false
	}

	return false, false, false
}

func (s *SchedulerImpl) areAllWfInWGAtBarrier(wg *WorkGroup) bool {
	for _, wf := range wg.waves() {
		if wf.execution().State != wavefront.WfAtBarrier {
			return false
		}
	}
	return true
}

func (s *SchedulerImpl) passBarrier(
	wg *WorkGroup,
) {
	s.removeAllWfFromBarrierBuffer(wg)
	s.setAllWfStateToReady(wg)
}

func (s *SchedulerImpl) setAllWfStateToReady(
	wg *WorkGroup,
) {
	for _, wf := range wg.waves() {
		s.cu.completeInstruction(wf, wf.DynamicInst())

		if executionEnded(wf.execution().State) {
			continue
		}

		s.cu.UpdatePCAndSetReady(wf)
	}
}

func (s *SchedulerImpl) removeAllWfFromBarrierBuffer(wg *WorkGroup) {
	newBarrierBuffer := make([]uint64, 0, s.barrierBufferSize)
	for _, id := range s.state().Barrier {
		wavefront := s.cu.wave(id)
		if wavefront.group().UID != wg.UID {
			newBarrierBuffer = append(newBarrierBuffer, id)
		}
	}
	s.state().Barrier = newBarrierBuffer
}

func (s *SchedulerImpl) removeAllWfFromInternalExecuting(
	wg *WorkGroup,
	internalExecuting *[]uint64,
) {
	newInternalExecuting := make([]uint64, 0)
	for _, id := range *internalExecuting {
		wavefront := s.cu.wave(id)
		if wavefront.group().UID != wg.UID {
			newInternalExecuting = append(newInternalExecuting, id)
		}
	}
	*internalExecuting = newInternalExecuting
}

func (s *SchedulerImpl) evalSWaitCnt(
	wf *Wavefront,
) (madeProgress bool, instCompleted bool) {
	done := true
	inst := wf.Inst()

	if wf.execution().OutstandingScalarMemAccess > inst.LKGMCNT {
		done = false
	}

	if wf.execution().OutstandingVectorMemAccess > inst.VMCNT {
		done = false
	}

	if done {
		// The outstanding memory accesses the S_WAITCNT was waiting on have
		// drained to the requested counts: mark the end of that wait.
		s.markMemDrained(wf, "s_waitcnt")
		s.cu.UpdatePCAndSetReady(wf)
		return true, true
	}

	return false, false
}

// markMemDrained records a "data" milestone on a wavefront's instruction task,
// marking the moment the memory accesses it was waiting on have drained.
func (s *SchedulerImpl) markMemDrained(wf *Wavefront, what string) {
	s.cu.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: s.cu.comp.State.InstTasks[wf.DynamicInst().ID],
		Kind:   tasks.MilestoneKindData,
		What:   what,
	})
}

func workGroupTimings(wg *WorkGroup) []protocol.WavefrontTiming {
	times := make([]protocol.WavefrontTiming, 0, len(wg.waves()))
	for _, w := range wg.waves() {
		times = append(
			times,
			protocol.WavefrontTiming{WavefrontID: w.UID,
				IssueTime: w.execution().SampleIssueTime, FinishTime: w.execution().SampleFinishTime},
		)
	}
	return times
}

func (s *SchedulerImpl) state() *SchedulerState { return &s.cu.comp.State.Scheduler }
