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
	Pause()
	Resume()
	Flush()
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
	if s.state().Paused == false {
		if s.scoreboardEnabled {
			madeProgress = s.tickScoreboards() || madeProgress
		}
		madeProgress = s.EvaluateInternalInst() || madeProgress
		madeProgress = s.DecodeNextInst() || madeProgress
		madeProgress = s.DoIssue() || madeProgress
		madeProgress = s.DoFetch() || madeProgress
	}
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
		for _, wf := range wfPool.waves() {
			if wf.ScoreboardData != nil {
				sb := wf.ScoreboardData.(*Scoreboard)
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
		for _, wf := range wfPool.waves() {
			if len(wf.InstBuffer) == 0 {
				wf.InstBufferStartPC = wf.PC() & 0xffffffffffffffc0
				continue
			}

			if wf.State != wavefront.WfReady {
				continue
			}

			if wf.NextInst() != nil {
				continue
			}

			if !s.wfHasAtLeast4BytesInInstBuffer(wf) {
				continue
			}

			inst, err := s.cu.Decoder.Decode(
				wf.InstBuffer[wf.PC()-wf.InstBufferStartPC:])
			if err == nil {
				next := wavefront.NewInst(s.cu.comp.NewID(), inst)
				start := wf.PC() - wf.InstBufferStartPC
				next.Raw = append([]byte(nil), wf.InstBuffer[start:start+uint64(inst.ByteSize)]...)
				wf.SetNextInst(next)
				madeProgress = true
			}
		}
	}
	return madeProgress
}

func (s *SchedulerImpl) wfHasAtLeast4BytesInInstBuffer(
	wf *wavefront.Wavefront,
) bool {
	return len(wf.InstBuffer[wf.PC()-wf.InstBufferStartPC:]) >= 4
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

		if len(wf.InstBuffer) == 0 {
			wf.InstBufferStartPC = wf.PC() & 0xffffffffffffffc0
		}
		addr := wf.InstBufferStartPC + uint64(len(wf.InstBuffer))
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

		wf.IsFetching = true

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
		if wf.InFlightInsts == 0 && !wfHasInstructionReady(wf) {
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

	if s.state().Paused == false {
		wfs := s.issueArbiter.Arbitrate(s.cu.WfPools)
		for _, wf := range wfs {
			if wf.NextInst().ExeUnit == insts.ExeUnitSpecial {
				madeProgress = s.issueToInternal(wf) || madeProgress

				continue
			}

			unit := s.getUnitToIssueTo(wf.NextInst().ExeUnit)
			if unit.CanAcceptWave() {
				wf.SetDynamicInst(wf.NextInst())
				wf.SetNextInst(nil)

				s.cu.logInstTask(wf, wf.DynamicInst(), false)

				if s.scoreboardEnabled && wf.ScoreboardData != nil {
					latency := GetScoreboardLatency(wf.DynamicInst().Inst)
					if latency > 0 {
						wf.ScoreboardData.(*Scoreboard).MarkBusy(
							wf.DynamicInst().Inst, latency)
					}
				}

				unit.AcceptWave(wf)
				wf.State = wavefront.WfRunning

				madeProgress = true
			}
		}
	}
	return madeProgress
}

func (s *SchedulerImpl) issueToInternal(wf *wavefront.Wavefront) bool {
	wf.SetDynamicInst(wf.NextInst())
	wf.SetNextInst(nil)
	s.state().Internal = append(s.state().Internal, s.cu.useWave(wf))
	wf.State = wavefront.WfRunning

	s.cu.logInstTask(wf, wf.DynamicInst(), false)

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

		switch executing.Inst().Opcode {
		case 1: // S_ENDPGM
			instProgress, instCompleted = s.evalSEndPgm(executing)
		case 10: // S_BARRIER
			instProgress, instCompleted, passBarrier = s.evalSBarrier(executing)

			if passBarrier {
				s.removeAllWfFromInternalExecuting(executing.WG, &newExecuting)
				s.removeAllWfFromInternalExecuting(
					executing.WG, &s.state().Internal)
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
			s.cu.logInstTask(executing, instruction, true)
		} else {
			newExecuting = append(newExecuting, id)
		}
	}

	s.state().Internal = newExecuting

	return madeProgress
}

func (s *SchedulerImpl) evalSEndPgm(
	wf *wavefront.Wavefront,
) (madeProgress bool, instCompleted bool) {
	if wf.OutstandingVectorMemAccess > 0 ||
		wf.OutstandingScalarMemAccess > 0 {
		return false, false
	}

	// sampling
	now := s.cu.CurrentTime()
	if s.cu.comp.Resources.Sampler != nil && !s.cu.comp.Resources.Sampler.IfDisable() {
		issuetime, found := s.cu.comp.State.WFTimes[wf.UID]
		if found {
			finishtime := now
			wf.SampleFinishTime = finishtime
			wf.SampleIssueTime = issuetime
			delete(s.cu.comp.State.WFTimes, wf.UID)
		}
	}

	allCompleted := s.areAllOtherWfsInWGCompleted(wf.WG, wf)

	// The only path that cannot retire this tick is the work-group-completing
	// one when the ACE port is full. Decide that here, before emitting the
	// milestone, so a retry does not re-emit it.
	if allCompleted && !s.cu.acePort().CanSend() {
		return false, false
	}

	// S_ENDPGM will retire this tick: mark the end of the drain wait exactly
	// once, before any branch below ends the inst task (the barrier and
	// executing paths close it via logInstTask).
	s.markMemDrained(wf, "s_endpgm")

	// Cap the wavefront's final execution burst with a work milestone before it
	// completes. The tail can keep InFlightInsts > 0 (e.g. a long outstanding
	// VMem) right up to S_ENDPGM, so no count->0 work milestone fires on its own;
	// without this the busy tail would be left unattributed.
	s.cu.markWfMilestone(wf, tasks.MilestoneKindWork, "execution")

	if allCompleted {
		s.sendWGCompletionMessage(wf.WG)

		wf.State = wavefront.WfCompleted

		s.resetRegisterValue(wf)
		s.cu.clearWGResource(wf.WG)

		s.cu.comp.TaskTracker().EndTask(s.cu.comp.State.WFTasks[wf.UID])
		delete(s.cu.comp.State.WFTasks, wf.UID)
		s.cu.completeWGTask(wf.WG.MapReq.ID)
	} else if s.areAllOtherWfsInWGAtBarrier(wf.WG, wf) {
		s.passBarrier(wf.WG)
		s.resetRegisterValue(wf)

		wf.State = wavefront.WfCompleted

		s.cu.comp.TaskTracker().EndTask(s.cu.comp.State.WFTasks[wf.UID])
		delete(s.cu.comp.State.WFTasks, wf.UID)
	} else if s.atLeaseOneWfIsExecuting(wf.WG) {
		s.resetRegisterValue(wf)

		wf.State = wavefront.WfCompleted

		s.cu.logInstTask(wf, wf.DynamicInst(), true)
		s.cu.comp.TaskTracker().EndTask(s.cu.comp.State.WFTasks[wf.UID])
		delete(s.cu.comp.State.WFTasks, wf.UID)
	} else {
		panic("never")
	}

	return true, true
}

func (s *SchedulerImpl) areAllOtherWfsInWGCompleted(
	wg *wavefront.WorkGroup,
	currWf *wavefront.Wavefront,
) bool {
	for _, wf := range wg.Wfs {
		if wf == currWf {
			continue
		}

		if wf.State != wavefront.WfCompleted {
			return false
		}
	}

	return true
}

func (s *SchedulerImpl) atLeaseOneWfIsExecuting(
	wg *wavefront.WorkGroup,
) bool {
	for _, wf := range wg.Wfs {
		if wf.State == wavefront.WfRunning || wf.State == wavefront.WfReady {
			return true
		}
	}

	return false
}

func (s *SchedulerImpl) sendWGCompletionMessage(
	wg *wavefront.WorkGroup,
) (done bool) {
	mapReq := wg.MapReq
	dispatcher := mapReq.Src

	msg := messaging.Msg{ID: s.cu.comp.NewID(),
		Src: s.cu.acePort().AsRemote(),
		Dst: dispatcher,
		Payload: protocol.WGCompletionMsg{RspToIDs: []uint64{mapReq.ID},
			Timings: workGroupTimings(wg)}}

	if !s.cu.acePort().CanSend() {
		return false
	}

	s.cu.acePort().Send(msg)

	return true
}

func (s *SchedulerImpl) areAllOtherWfsInWGAtBarrier(
	wg *wavefront.WorkGroup,
	currWf *wavefront.Wavefront,
) bool {
	for _, wf := range wg.Wfs {
		if wf == currWf {
			continue
		}

		if wf.State != wavefront.WfAtBarrier &&
			wf.State != wavefront.WfCompleted {
			return false
		}
	}

	return true
}

func (s *SchedulerImpl) resetRegisterValue(wf *wavefront.Wavefront) {
	if wf.CodeObject.WIVgprCount > 0 {
		vRegFile := s.cu.VRegFile[wf.SIMDID].(*SimpleRegisterFile)
		vRegStorage := vRegFile.bytes()
		data := make([]byte, wf.CodeObject.WIVgprCount*4)
		for i := 0; i < 64; i++ {
			offset := uint64(wf.VRegOffset + vRegFile.ByteSizePerLane*i)
			copy(vRegStorage[offset:], data)
		}
	}

	if wf.CodeObject.WFSgprCount > 0 {
		sRegFile := s.cu.SRegFile.(*SimpleRegisterFile)
		sRegStorage := sRegFile.bytes()
		data := make([]byte, wf.CodeObject.WFSgprCount*4)
		offset := uint64(wf.SRegOffset)
		copy(sRegStorage[offset:], data)
	}
}

func (s *SchedulerImpl) evalSBarrier(
	wf *wavefront.Wavefront,
) (madeProgress bool, instCompleted bool, passBarrier bool) {
	wf.State = wavefront.WfAtBarrier

	wg := wf.WG
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

func (s *SchedulerImpl) areAllWfInWGAtBarrier(wg *wavefront.WorkGroup) bool {
	for _, wf := range wg.Wfs {
		if wf.State != wavefront.WfAtBarrier {
			return false
		}
	}
	return true
}

func (s *SchedulerImpl) passBarrier(
	wg *wavefront.WorkGroup,
) {
	s.removeAllWfFromBarrierBuffer(wg)
	s.setAllWfStateToReady(wg)
}

func (s *SchedulerImpl) setAllWfStateToReady(
	wg *wavefront.WorkGroup,
) {
	for _, wf := range wg.Wfs {
		s.cu.logInstTask(wf, wf.DynamicInst(), true)

		if wf.State == wavefront.WfCompleted {
			continue
		}

		s.cu.UpdatePCAndSetReady(wf)
	}
}

func (s *SchedulerImpl) removeAllWfFromBarrierBuffer(wg *wavefront.WorkGroup) {
	newBarrierBuffer := make([]uint64, 0, s.barrierBufferSize)
	for _, id := range s.state().Barrier {
		wavefront := s.cu.wave(id)
		if wavefront.WG != wg {
			newBarrierBuffer = append(newBarrierBuffer, id)
		}
	}
	s.state().Barrier = newBarrierBuffer
}

func (s *SchedulerImpl) removeAllWfFromInternalExecuting(
	wg *wavefront.WorkGroup,
	internalExecuting *[]uint64,
) {
	newInternalExecuting := make([]uint64, 0)
	for _, id := range *internalExecuting {
		wavefront := s.cu.wave(id)
		if wavefront.WG != wg {
			newInternalExecuting = append(newInternalExecuting, id)
		}
	}
	*internalExecuting = newInternalExecuting
}

func (s *SchedulerImpl) evalSWaitCnt(
	wf *wavefront.Wavefront,
) (madeProgress bool, instCompleted bool) {
	done := true
	inst := wf.Inst()

	if wf.OutstandingScalarMemAccess > inst.LKGMCNT {
		done = false
	}

	if wf.OutstandingVectorMemAccess > inst.VMCNT {
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
func (s *SchedulerImpl) markMemDrained(wf *wavefront.Wavefront, what string) {
	s.cu.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: s.cu.comp.State.InstTasks[wf.DynamicInst().ID],
		Kind:   tasks.MilestoneKindData,
		What:   what,
	})
}

// Pause pauses
func (s *SchedulerImpl) Pause() {
	s.state().Paused = true
}

// Resume resumes
func (s *SchedulerImpl) Resume() {
	s.state().Paused = false
}

// Flush flushes
func (s *SchedulerImpl) Flush() {
	s.state().Barrier = nil
	s.state().Internal = nil
}

func workGroupTimings(wg *wavefront.WorkGroup) []protocol.WavefrontTiming {
	times := make([]protocol.WavefrontTiming, 0, len(wg.Wfs))
	for _, w := range wg.Wfs {
		times = append(
			times,
			protocol.WavefrontTiming{WavefrontID: w.UID, IssueTime: w.SampleIssueTime, FinishTime: w.SampleFinishTime},
		)
	}
	return times
}

func (s *SchedulerImpl) state() *SchedulerState { return &s.cu.comp.State.Scheduler }
