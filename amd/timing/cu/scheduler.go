package cu

import (
	"log"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/tracing"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/protocol"
	"github.com/sarchlab/mgpusim/v4/amd/sampling"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
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
	cu                *ComputeUnit
	fetchArbiter      WfArbiter
	issueArbiter      WfArbiter
	internalExecuting []*wavefront.Wavefront

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

// Run runs scheduler
func (s *SchedulerImpl) Run() bool {
	madeProgress := false
	if s.isPaused == false {
		if s.EvaluateInternalInst() {
			madeProgress = true
		}
		if s.DecodeNextInst() {
			madeProgress = true
		}
		if s.DoIssue() {
			madeProgress = true
		}

		// Inject S_ENDPGM for all wavefronts that have reached the end of the kernel binary
		if s.injectSEndPgmForCompletedWavefronts() {
			madeProgress = true
		}

		madeProgress = s.DoFetch() || madeProgress
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

// DecodeNextInst checks
func (s *SchedulerImpl) DecodeNextInst() bool {
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

			// Check if this wavefront has reached the end of the kernel binary
			if wf.PC >= wf.InstBufferStartPC+uint64(len(wf.InstBuffer)) {
				// Create a fake S_ENDPGM instruction
				fakeInst := &insts.Inst{
					InstType: &insts.InstType{
						Opcode:  1, // S_ENDPGM opcode
						Format:  &insts.Format{FormatType: insts.SOPP},
						ExeUnit: insts.ExeUnitSpecial,
					},
				}
				wf.InstToIssue = wavefront.NewInst(fakeInst)
				madeProgress = true
				continue
			}

			if !s.wfHasAtLeast4BytesInInstBuffer(wf) {
				continue
			}

			inst, err := s.cu.Decoder.Decode(
				wf.InstBuffer[wf.PC-wf.InstBufferStartPC:])
			if err == nil {
				wf.InstToIssue = wavefront.NewInst(inst)
				// s.cu.logInstTask(now, wf, wf.InstToIssue, false)
				madeProgress = true
			}
		}
	}
	return madeProgress
}

func (s *SchedulerImpl) wfHasAtLeast4BytesInInstBuffer(wf *wavefront.Wavefront) bool {
	return len(wf.InstBuffer[wf.PC-wf.InstBufferStartPC:]) >= 4
}

// DoFetch function of the scheduler will fetch instructions from the
// instruction memory
func (s *SchedulerImpl) DoFetch() bool {
	madeProgress := false
	wfs := s.fetchArbiter.Arbitrate(s.cu.WfPools)

	// Debug: Check if there are any wavefronts in the pools
	totalWfs := 0
	for _, pool := range s.cu.WfPools {
		totalWfs += len(pool.wfs)
	}
	if totalWfs > 0 && len(wfs) == 0 {
		// No wavefronts available for fetch
	}

	if len(wfs) > 0 {
		wf := wfs[0]

		if len(wf.InstBuffer) == 0 {
			wf.InstBufferStartPC = wf.PC & 0xffffffffffffffc0
		}
		addr := wf.InstBufferStartPC + uint64(len(wf.InstBuffer))
		addr = addr & 0xffffffffffffffc0
		req := mem.ReadReqBuilder{}.
			WithSrc(s.cu.ToInstMem.AsRemote()).
			WithDst(s.cu.InstMem.AsRemote()).
			WithAddress(addr).
			WithPID(wf.PID()).
			WithByteSize(64).
			Build()

		err := s.cu.ToInstMem.Send(req)
		if err == nil {
			info := new(InstFetchReqInfo)
			info.Wavefront = wf
			info.Req = req
			info.Address = addr
			s.cu.InFlightInstFetch = append(s.cu.InFlightInstFetch, info)
			wf.IsFetching = true

			madeProgress = true

			tracing.StartTask(req.ID+"_fetch", wf.UID,
				s.cu, "fetch", "fetch", nil)
			tracing.TraceReqInitiate(req, s.cu, req.ID+"_fetch")
		}
	}

	return madeProgress
}

// DoIssue function of the scheduler issues fetched instruction to the decoding
// units
func (s *SchedulerImpl) DoIssue() bool {
	madeProgress := false

	if s.isPaused == false {
		wfs := s.issueArbiter.Arbitrate(s.cu.WfPools)
		for _, wf := range wfs {
			if wf.InstToIssue.ExeUnit == insts.ExeUnitSpecial {
				madeProgress = s.issueToInternal(wf) || madeProgress

				continue
			}

			unit := s.getUnitToIssueTo(wf.InstToIssue.ExeUnit)
			if unit.CanAcceptWave() {
				wf.SetDynamicInst(wf.InstToIssue)
				wf.InstToIssue = nil

				s.cu.logInstTask(wf, wf.DynamicInst(), false)

				unit.AcceptWave(wf)
				wf.State = wavefront.WfRunning
				//s.removeStaleInstBuffer(wf)

				madeProgress = true
			}
		}
	}
	return madeProgress
}

func (s *SchedulerImpl) issueToInternal(wf *wavefront.Wavefront) bool {
	wf.SetDynamicInst(wf.InstToIssue)
	wf.InstToIssue = nil
	s.internalExecuting = append(s.internalExecuting, wf)
	wf.State = wavefront.WfRunning
	//s.removeStaleInstBuffer(wf)

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
	if s.internalExecuting == nil {
		return false
	}

	madeProgress := false

	newExecuting := make([]*wavefront.Wavefront, 0)
	for _, executing := range s.internalExecuting {
		instProgress := false
		instCompleted := false
		passBarrier := false

		opcode := executing.Inst().Opcode
		switch opcode {
		case 1: // S_ENDPGM
			instProgress, instCompleted = s.evalSEndPgm(executing)
		case 10: // S_BARRIER
			instProgress, instCompleted, passBarrier = s.evalSBarrier(executing)

			if passBarrier {
				s.removeAllWfFromInternalExecuting(executing.WG, &newExecuting)
				s.removeAllWfFromInternalExecuting(executing.WG, &s.internalExecuting)
			}
		case 12: // S_WAITCNT
			instProgress, instCompleted = s.evalSWaitCnt(executing)
		default:
			// The program has to make progress
			executing.State = wavefront.WfReady
			instProgress = true
			instCompleted = true
		}
		madeProgress = instProgress || madeProgress

		if instCompleted {
			if executing.DynamicInst() != nil {
				s.cu.logInstTask(executing, executing.DynamicInst(), true)
			}
		} else {
			newExecuting = append(newExecuting, executing)
		}
	}

	s.internalExecuting = newExecuting

	return madeProgress
}

func (s *SchedulerImpl) evalSEndPgm(
	wf *wavefront.Wavefront,
) (madeProgress bool, instCompleted bool) {
	if wf.OutstandingVectorMemAccess > 0 ||
		wf.OutstandingScalarMemAccess > 0 {
		return false, false
	}

	////sampling
	now := s.cu.CurrentTime()
	if *sampling.SampledRunnerFlag {
		issuetime, found := s.cu.wftime[wf.UID]
		if found {
			finishtime := now
			wf.FinishTime = finishtime
			wf.IssueTime = issuetime
			delete(s.cu.wftime, wf.UID)
		}
	}
	if s.areAllOtherWfsInWGCompleted(wf.WG, wf) {
		done := s.sendWGCompletionMessage(wf.WG)
		if !done {
			return false, false
		}

		wf.State = wavefront.WfCompleted

		s.resetRegisterValue(wf)
		s.cu.clearWGResource(wf.WG)

		tracing.EndTask(wf.UID, s.cu)
		tracing.TraceReqComplete(wf.WG.MapReq, s.cu)

		return true, true
	}

	if s.areAllOtherWfsInWGAtBarrier(wf.WG, wf) {
		s.passBarrier(wf.WG)
		s.resetRegisterValue(wf)

		wf.State = wavefront.WfCompleted

		tracing.EndTask(wf.UID, s.cu)

		return true, true
	}

	if s.atLeaseOneWfIsExecuting(wf.WG) {
		s.resetRegisterValue(wf)

		wf.State = wavefront.WfCompleted

		s.cu.logInstTask(wf, wf.DynamicInst(), true)
		tracing.EndTask(wf.UID, s.cu)

		return true, true
	}

	panic("never")
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

	msg := protocol.WGCompletionMsgBuilder{}.
		WithSrc(s.cu.ToCP.AsRemote()).
		WithDst(dispatcher).
		WithRspTo([]string{mapReq.ID}).
		Build()

	err := s.cu.ToCP.Send(msg)

	return err == nil
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

	if len(s.barrierBuffer) < s.barrierBufferSize {
		s.barrierBuffer = append(s.barrierBuffer, wf)
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
	newBarrierBuffer := make([]*wavefront.Wavefront, 0, s.barrierBufferSize)
	for _, wavefront := range s.barrierBuffer {
		if wavefront.WG != wg {
			newBarrierBuffer = append(newBarrierBuffer, wavefront)
		}
	}
	s.barrierBuffer = newBarrierBuffer
}

func (s *SchedulerImpl) removeAllWfFromInternalExecuting(
	wg *wavefront.WorkGroup,
	internalExecuting *[]*wavefront.Wavefront,
) {
	newInternalExecuting := make([]*wavefront.Wavefront, 0)
	for _, wavefront := range *internalExecuting {
		if wavefront.WG != wg {
			newInternalExecuting = append(newInternalExecuting, wavefront)
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
		s.cu.UpdatePCAndSetReady(wf)
		return true, true
	}

	return false, false
}

// Pause pauses
func (s *SchedulerImpl) Pause() {
	s.isPaused = true
}

// Resume resumes
func (s *SchedulerImpl) Resume() {
	s.isPaused = false
}

// Flush flushes
func (s *SchedulerImpl) Flush() {
	s.barrierBuffer = nil
	s.internalExecuting = nil
}

// injectSEndPgmForCompletedWavefronts injects S_ENDPGM for all wavefronts that have reached the end of the kernel binary
func (s *SchedulerImpl) injectSEndPgmForCompletedWavefronts() bool {
	madeProgress := false

	// Check all wavefront pools
	for _, pool := range s.cu.WfPools {
		for _, wf := range pool.wfs {
			// Skip if wavefront is already completed or has an instruction to issue
			if wf.State == wavefront.WfCompleted || wf.InstToIssue != nil {
				continue
			}

			// Check if wavefront has reached the end of the kernel binary
			if wf.CodeObject != nil && wf.CodeObject.Symbol != nil {
				lastPCInBinary := wf.CodeObject.Symbol.Size + wf.WG.Packet.KernelObject + wf.CodeObject.KernelCodeEntryByteOffset
				lastPCInInstBuffer := wf.InstBufferStartPC + uint64(len(wf.InstBuffer))

				if lastPCInInstBuffer >= lastPCInBinary {
					// Create a fake S_ENDPGM instruction
					fakeInst := &insts.Inst{
						InstType: &insts.InstType{
							Opcode:  1, // S_ENDPGM opcode
							Format:  &insts.Format{FormatType: insts.SOPP},
							ExeUnit: insts.ExeUnitSpecial,
						},
					}
					wf.InstToIssue = wavefront.NewInst(fakeInst)
					madeProgress = true
				}
			}
		}
	}

	return madeProgress
}
