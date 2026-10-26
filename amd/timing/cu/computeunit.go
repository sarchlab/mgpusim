package cu

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// A ComputeUnit provides a detailed and accurate simulation of a GCN3
// ComputeUnit. It is the (single) middleware of the timing compute-unit
// component: its Tick method reproduces the v4 ComputeUnit.Tick body, and it
// also serves as the timing.Handler for the CU's custom events
// (WfCompletionEvent, WfDispatchEvent).
//
// Mutable execution values belong to comp.State; the middleware holds only
// behavior, fixed dependencies, and derived identity views.
type ComputeUnit struct {
	comp   *Comp
	engine timing.EventScheduler

	// Handler IDs that the custom events of this CU dispatch on.
	wfCompletionHandlerID string
	wfDispatchHandlerID   string

	WfDispatcher WfDispatcher
	Decoder      emu.Decoder
	WfPools      []*WavefrontPool

	InFlightVectorMemAccessLimit int

	Scheduler        Scheduler
	BranchUnit       SubComponent
	VectorMemDecoder SubComponent
	VectorMemUnit    SubComponent
	ScalarDecoder    SubComponent
	VectorDecoder    SubComponent
	LDSDecoder       SubComponent
	ScalarUnit       SubComponent
	SIMDUnit         []SubComponent
	LDSUnit          SubComponent
	SRegFile         RegisterFile
	VRegFile         []RegisterFile

	// Port instances, resolved lazily from the component (ports are assigned
	// externally after Build). Tests may set these fields directly.
	ToACE       messaging.Port
	ToInstMem   messaging.Port
	ToScalarMem messaging.Port
	ToVectorMem messaging.Port
	ToCP        messaging.Port

	// Derived identity views. State owns their execution values.
	workGroups   map[uint64]*wavefront.WorkGroup
	waves        map[uint64]*wavefront.Wavefront
	instructions map[uint64]*wavefront.Inst
	boundDecoder *DecoderState
}

// Comp returns the component this middleware belongs to.
func (cu *ComputeUnit) Comp() *Comp {
	return cu.comp
}

func (cu *ComputeUnit) acePort() messaging.Port {
	if cu.ToACE == nil {
		cu.ToACE = cu.comp.Ports.Top
	}

	return cu.ToACE
}

func (cu *ComputeUnit) instMemPort() messaging.Port {
	if cu.ToInstMem == nil {
		cu.ToInstMem = cu.comp.Ports.InstMem
	}

	return cu.ToInstMem
}

func (cu *ComputeUnit) scalarMemPort() messaging.Port {
	if cu.ToScalarMem == nil {
		cu.ToScalarMem = cu.comp.Ports.ScalarMem
	}

	return cu.ToScalarMem
}

func (cu *ComputeUnit) vectorMemPort() messaging.Port {
	if cu.ToVectorMem == nil {
		cu.ToVectorMem = cu.comp.Ports.VectorMem
	}

	return cu.ToVectorMem
}

func (cu *ComputeUnit) cpPort() messaging.Port {
	if cu.ToCP == nil {
		cu.ToCP = cu.comp.Ports.Ctrl
	}

	return cu.ToCP
}

// CurrentTime returns the current simulation time.
func (cu *ComputeUnit) CurrentTime() timing.VTimeInPicoSec {
	return cu.comp.CurrentTime()
}

// Tick ticks. The order of the four phases reproduces the v4
// ComputeUnit.Tick body exactly.
func (cu *ComputeUnit) Tick() bool {
	cu.ensureState()
	madeProgress := false

	madeProgress = cu.runPipeline() || madeProgress
	madeProgress = cu.sendToCP() || madeProgress
	madeProgress = cu.processInput() || madeProgress
	madeProgress = cu.doFlush() || madeProgress
	cu.retireGroups()

	return madeProgress
}

//nolint:gocyclo
func (cu *ComputeUnit) runPipeline() bool {
	madeProgress := false

	if !cu.comp.State.IsPaused {
		madeProgress = cu.BranchUnit.Run() || madeProgress
		madeProgress = cu.ScalarUnit.Run() || madeProgress
		madeProgress = cu.ScalarDecoder.Run() || madeProgress
		for _, simdUnit := range cu.SIMDUnit {
			madeProgress = simdUnit.Run() || madeProgress
		}
		madeProgress = cu.VectorDecoder.Run() || madeProgress
		madeProgress = cu.LDSUnit.Run() || madeProgress
		madeProgress = cu.LDSDecoder.Run() || madeProgress
		madeProgress = cu.VectorMemUnit.Run() || madeProgress
		madeProgress = cu.VectorMemDecoder.Run() || madeProgress
		madeProgress = cu.Scheduler.Run() || madeProgress
	}

	return madeProgress
}

func (cu *ComputeUnit) doFlush() bool {
	madeProgress := false
	if cu.comp.State.IsFlushing {
		// If a flush request arrives before the shadow buffer requests have
		// been sent out.
		if cu.comp.State.IsSendingOutShadowBufferReqs {
			madeProgress =
				cu.reInsertShadowBufferReqsToOriginalBuffers() || madeProgress
		}
		madeProgress = cu.flushPipeline() || madeProgress
	}

	if cu.comp.State.IsSendingOutShadowBufferReqs {
		madeProgress = cu.checkShadowBuffers() || madeProgress
	}

	return madeProgress
}

func (cu *ComputeUnit) processInput() bool {
	madeProgress := false

	state := &cu.comp.State
	if !state.IsPaused || state.IsSendingOutShadowBufferReqs {
		madeProgress = cu.processInputFromACE() || madeProgress
		madeProgress = cu.processInputFromInstMem() || madeProgress
		madeProgress = cu.processInputFromScalarMem() || madeProgress
		madeProgress = cu.processInputFromVectorMem() || madeProgress
	}

	madeProgress = cu.processInputFromCP() || madeProgress

	return madeProgress
}

func (cu *ComputeUnit) processInputFromCP() bool {
	req, ok := cu.cpPort().PeekIncoming()
	if !ok {
		return false
	}

	switch req.Payload.(type) {
	case protocol.CUPipelineRestartReq:

		return cu.handlePipelineResume(req)
	case protocol.CUPipelineFlushReq:

		return cu.handlePipelineFlushReq(req)
	default:
		panic("unknown msg type")
	}
}

func (cu *ComputeUnit) handlePipelineFlushReq(
	req messaging.Msg,
) bool {
	state := &cu.comp.State
	state.IsFlushing = true
	state.HasFlushReq = true
	state.FlushReqID = req.ID
	state.FlushReqSrc = req.Src

	cu.cpPort().RetrieveIncoming()

	return true
}

func (cu *ComputeUnit) handlePipelineResume(
	req messaging.Msg,
) bool {
	// v4 sent the restart response unconditionally and panicked when the
	// port was full. v5 ports panic on Send when full, so the request stays
	// queued until the response can go out.
	if !cu.cpPort().CanSend() {
		return false
	}

	state := &cu.comp.State
	state.IsSendingOutShadowBufferReqs = true

	rsp := messaging.Msg{ID: cu.comp.NewID(),
		Src:   cu.cpPort().AsRemote(),
		Dst:   req.Src,
		RspTo: req.ID, Payload: protocol.CUPipelineRestartRsp{}}

	cu.cpPort().Send(rsp)

	cu.cpPort().RetrieveIncoming()

	return true
}

func (cu *ComputeUnit) sendToCP() bool {
	state := &cu.comp.State
	if !state.HasPendingCPRsp {
		return false
	}

	if !cu.cpPort().CanSend() {
		return false
	}

	rsp := messaging.Msg{ID: cu.comp.NewID(),
		Src:   cu.cpPort().AsRemote(),
		Dst:   state.PendingCPRspDst,
		RspTo: state.PendingCPRspTo, Payload: protocol.CUPipelineFlushRsp{}}

	cu.cpPort().Send(rsp)

	state.HasPendingCPRsp = false

	return true
}

func (cu *ComputeUnit) flushPipeline() bool {
	state := &cu.comp.State

	if !state.HasFlushReq {
		return false
	}

	if state.IsHandlingWfCompletionEvent {
		return false
	}

	cu.comp.State.ShadowInstFetch = nil
	cu.comp.State.ShadowScalar = nil
	cu.comp.State.ShadowVector = nil

	cu.populateShadowBuffers()
	cu.endInflightTracingTasks()
	cu.setWavesToReady()
	cu.Scheduler.Flush()
	cu.flushInternalComponents()
	cu.Scheduler.Pause()
	state.IsPaused = true

	state.HasPendingCPRsp = true
	state.PendingCPRspDst = state.FlushReqSrc
	state.PendingCPRspTo = state.FlushReqID
	state.HasFlushReq = false
	state.IsFlushing = false

	return true
}

func (cu *ComputeUnit) flushInternalComponents() {
	cu.BranchUnit.Flush()

	cu.ScalarUnit.Flush()
	cu.ScalarDecoder.Flush()

	for _, simdUnit := range cu.SIMDUnit {
		simdUnit.Flush()
	}

	cu.VectorDecoder.Flush()
	cu.LDSUnit.Flush()
	cu.LDSDecoder.Flush()
	cu.VectorMemDecoder.Flush()
	cu.VectorMemUnit.Flush()
}

// endInflightTracingTasks ends the tracing tasks that a pipeline flush drops,
// so they do not leak as started-never-ended. A flush resets every running
// wavefront to WfReady (setWavesToReady) and clears the sub-unit pipeline
// stages and the scheduler's internalExecuting slice; an instruction sitting in
// one of those stages is re-issued from scratch (with a new ID) after the
// restart, so its "inst" task would otherwise never end. Such a wavefront is in
// the WfRunning state, so ending the inst task of every running wavefront
// covers all sub-unit stages and internalExecuting uniformly.
//
// In-flight memory accesses are not dropped: they are preserved in the shadow
// buffers and re-sent, and their inst tasks end when the (shadowed) response
// returns. Those wavefronts have already advanced past the issuing instruction
// and sit in WfReady, so the WfRunning filter correctly skips them and the inst
// task is not double-ended. The shadowed accesses' req_out tasks are ended in
// populateShadowBuffers, where the re-send regenerates the message ID.
func (cu *ComputeUnit) endInflightTracingTasks() {
	for _, wfPool := range cu.WfPools {
		for _, wf := range wfPool.waves() {
			if wf.State != wavefront.WfRunning {
				continue
			}

			if inst := wf.DynamicInst(); inst != nil {
				// This running inst's task is force-ended here, bypassing
				// logInstTask, so keep the in-flight counter balanced.
				if wf.InFlightInsts > 0 {
					wf.InFlightInsts--
				}
				cu.comp.TaskTracker().EndTask(cu.comp.State.InstTasks[inst.ID])
				delete(cu.comp.State.InstTasks, inst.ID)
				cu.releaseInstruction(inst.ID)
			}
		}
	}
}

func (cu *ComputeUnit) processInputFromACE() bool {
	req, ok := cu.acePort().RetrieveIncoming()
	if !ok {
		return false
	}

	switch req.Payload.(type) {
	case protocol.MapWGReq:

		return cu.handleMapWGReq(req)
	default:
		panic("unknown req type")
	}
}

// Handle processes the custom events of the compute unit (the regular tick
// events are dispatched to the component's own handler).
func (cu *ComputeUnit) Handle(evt timing.Event) {
	cu.ensureState()
	ctx := hooking.HookCtx{
		Domain: cu.comp,
		Pos:    timing.HookPosBeforeEvent,
		Item:   evt,
	}
	cu.comp.InvokeHook(ctx)

	switch evt := evt.(type) {
	case wavefront.WfCompletionEvent:
		cu.handleWfCompletionEvent(evt)
	case WfDispatchEvent:
		// The v4 code defined this event but never scheduled it. The handler
		// is registered for completeness; nothing needs to happen.
	default:
		log.Panicf("Unable to process event of type %s",
			reflect.TypeOf(evt))
	}

	ctx.Pos = timing.HookPosAfterEvent
	cu.comp.InvokeHook(ctx)
}

func (cu *ComputeUnit) handleWfCompletionEvent(
	evt wavefront.WfCompletionEvent,
) {
	wf := cu.wavefrontByID(evt.WavefrontID)
	wf.State = wavefront.WfCompleted
	s := cu.Scheduler.(*SchedulerImpl)

	if !s.areAllOtherWfsInWGCompleted(wf.WG, wf) {
		return
	}

	now := evt.Time()

	done := s.sendWGCompletionMessage(wf.WG)
	if !done {
		newEvent := wavefront.NewWfCompletionEvent(
			cu.comp.NewID(),
			cu.comp.Spec.Freq.NextTick(now), cu.wfCompletionHandlerID, wf.UID)
		cu.engine.Schedule(newEvent)

		return
	}

	s.resetRegisterValue(wf)
	cu.clearWGResource(wf.WG)

	// This path only runs for sampled wavefronts, each of which opened its own
	// "wavefront" task in handleMapWGReq. The work-group completes as a unit
	// here, so end every wavefront's task, not just the one whose completion
	// event fired — otherwise the other wavefronts' tasks leak.
	for _, wgWf := range wf.WG.Wfs {
		cu.comp.TaskTracker().EndTask(cu.comp.State.WFTasks[wgWf.UID])
		delete(cu.comp.State.WFTasks, wgWf.UID)
	}
	cu.completeWGTask(wf.WG.MapReq.ID)
	// A sampled completion may be the last event, with no subsequent CU tick.
	cu.retireGroups()
}

func (cu *ComputeUnit) handleMapWGReq(
	req messaging.Msg,
) bool {
	now := cu.CurrentTime()

	raw, err := req.Payload.(protocol.MapWGReq).WorkGroup.Restore()
	if err != nil {
		panic(err)
	}
	wg := cu.wrapWG(raw, req)
	cu.workGroups[wg.UID] = wg

	cu.comp.State.WGTasks[req.ID] = cu.comp.TaskTracker().StartReqIn(cu.comp, req)

	skipSimulate := cu.scheduleSampledWaves(wg, now)

	if !skipSimulate {
		for i, wf := range wg.Wfs {
			location := req.Payload.(protocol.MapWGReq).Wavefronts[i]
			cu.WfPools[location.SIMDID].AddWf(wf)
			cu.WfDispatcher.DispatchWf(wf, req.Payload.(protocol.MapWGReq).Wavefronts[i])
			wf.State = wavefront.WfReady

			cu.comp.State.WFTasks[wf.UID] = cu.comp.TaskTracker().StartTask(cu.comp, tasks.StartInfo{
				ParentID: cu.comp.State.WGTasks[req.ID],
				Kind:     "wavefront",
				What:     "wavefront",
				Location: cu.comp.Name() + ".WFPool",
			})
		}
	}

	cu.comp.State.Running = true
	cu.comp.TickLater()

	return true
}

func (cu *ComputeUnit) scheduleSampledWaves(wg *wavefront.WorkGroup, now timing.VTimeInPicoSec) bool {
	// sampling
	skipSimulate := false
	if cu.comp.Resources.Sampler != nil && !cu.comp.Resources.Sampler.IfDisable() {
		for _, wf := range wg.Wfs {
			cu.comp.State.WFTimes[wf.UID] = now
		}

		wfpredicttime, wfsampled := cu.comp.Resources.Sampler.Predict()
		predtime := wfpredicttime
		skipSimulate = wfsampled

		for _, wf := range wg.Wfs {
			if skipSimulate {
				predictedTime := predtime + now
				wf.State = wavefront.WfSampledCompleted
				newEvent := wavefront.NewWfCompletionEvent(
					cu.comp.NewID(),
					predictedTime, cu.wfCompletionHandlerID, wf.UID)
				cu.engine.Schedule(newEvent)
				cu.comp.State.WFTasks[wf.UID] = cu.comp.TaskTracker().StartTask(cu.comp, tasks.StartInfo{
					ParentID: cu.comp.State.WGTasks[wg.MapReq.ID],
					Kind:     "wavefront",
					What:     "wavefront",
					// The sampled fast path forgot the location, so the wavefront task
					// fell back to the bare CU name and collided with fetch. Pin it to
					// the same .WFPool location the normal path uses.
					Location: cu.comp.Name() + ".WFPool",
				})
			}
		}
	}

	return skipSimulate
}

func (cu *ComputeUnit) clearWGResource(wg *wavefront.WorkGroup) {
	g := cu.comp.State.Groups[wg.UID]
	g.Completed = true
	cu.comp.State.Groups[wg.UID] = g
	for _, wf := range wg.Wfs {
		wfPool := cu.WfPools[wf.SIMDID]
		wfPool.RemoveWf(wf)
	}
}

func (cu *ComputeUnit) isAllWfInWGCompleted(wg *wavefront.WorkGroup) bool {
	for _, wf := range wg.Wfs {
		if wf.State != wavefront.WfCompleted {
			return false
		}
	}
	return true
}

func (cu *ComputeUnit) hasMoreWfsToRun() bool {
	for _, wfpool := range cu.WfPools {
		if len(wfpool.waves()) > 0 {
			return true
		}
	}
	return false
}

func (cu *ComputeUnit) wrapWG(
	raw *kernels.WorkGroup,
	req messaging.Msg,
) *wavefront.WorkGroup {
	return cu.newGroup(raw, req)
}

func (cu *ComputeUnit) processInputFromInstMem() bool {
	rsp, ok := cu.instMemPort().RetrieveIncoming()
	if !ok {
		return false
	}

	switch rsp.Payload.(type) {
	case memprotocol.DataReadyRsp:

		cu.handleFetchReturn(rsp)
	default:

		log.Panicf("cannot handle request of type %s from InstMem port",
			reflect.TypeOf(rsp))
	}
	return true
}

func (cu *ComputeUnit) handleFetchReturn(
	rsp messaging.Msg,
) bool {
	matchIdx := -1
	for i, info := range cu.comp.State.InstFetch {
		if info.Req.ID == rsp.RspTo {
			matchIdx = i
			break
		}
	}
	if matchIdx < 0 {
		return false
	}

	info := cu.comp.State.InstFetch[matchIdx]
	wf := cu.wave(info.WF)
	addr := info.Address
	wasWaitingForFetch := wf.InFlightInsts == 0 && !wfHasInstructionReady(wf)
	cu.comp.State.InstFetch = append(
		cu.comp.State.InstFetch[:matchIdx],
		cu.comp.State.InstFetch[matchIdx+1:]...)

	if addr == wf.InstBufferStartPC+uint64(len(wf.InstBuffer)) {
		wf.InstBuffer = append(wf.InstBuffer, rsp.Payload.(memprotocol.DataReadyRsp).Data...)
	}

	wf.IsFetching = false
	wf.LastFetchTime = cu.CurrentTime()

	if wasWaitingForFetch {
		// The fetched instruction bytes arrived while nothing was executing: the
		// interval since the fetch went out was spent waiting for this data.
		cu.markWfMilestone(wf, tasks.MilestoneKindData, "inst_fetch")
	}

	cu.completeRequest(info.Req.ID)
	cu.comp.TaskTracker().EndTask(info.Task)
	return true
}

func (cu *ComputeUnit) processInputFromScalarMem() bool {
	rsp, ok := cu.scalarMemPort().RetrieveIncoming()
	if !ok {
		return false
	}

	switch rsp.Payload.(type) {
	case memprotocol.DataReadyRsp:

		cu.handleScalarDataLoadReturn(rsp)
	default:

		log.Panicf("cannot handle request of type %s from ScalarMem port",
			reflect.TypeOf(rsp))
	}
	return true
}

func (cu *ComputeUnit) handleScalarDataLoadReturn(
	rsp messaging.Msg,
) {
	matchIdx := -1
	for i, info := range cu.comp.State.Scalar {
		if info.Req.ID == rsp.RspTo {
			matchIdx = i
			break
		}
	}
	if matchIdx < 0 {
		return
	}

	info := cu.comp.State.Scalar[matchIdx]
	req := info.Req

	wf := cu.wave(info.WF)
	access := RegisterAccess{
		WaveOffset: wf.SRegOffset,
		Reg:        insts.Regs[info.Register],
		RegCount:   len(rsp.Payload.(memprotocol.DataReadyRsp).Data) / 4,
		Data:       rsp.Payload.(memprotocol.DataReadyRsp).Data,
	}
	cu.SRegFile.Write(access)

	cu.comp.State.Scalar = append(
		cu.comp.State.Scalar[:matchIdx],
		cu.comp.State.Scalar[matchIdx+1:]...)

	cu.completeRequest(req.ID)

	// Coalesced responses can return out of order, so the last request
	// generated is not necessarily the last received. Release the s_waitcnt
	// counter, end the inst task and mark the data wait only once no access
	// for this instruction is still in flight; releasing the counter earlier
	// lets the wavefront reuse the destination SGPRs before the remaining
	// responses overwrite them.
	if !cu.hasInFlightScalarMemFor(cu.instruction(info.Inst)) {
		wf.OutstandingScalarMemAccess--
		cu.markInstDataReturned(cu.instruction(info.Inst), "smem")
		cu.logInstTask(wf, cu.instruction(info.Inst), true)
	}
}

// markInstDataReturned records a "data" milestone on a memory instruction's
// task when its last memory response returns. The instruction's task ends at
// that same moment, so this closes the otherwise-unattributed gap between the
// memory request going out (or the in-flight slot being acquired) and the data
// coming back.
func (cu *ComputeUnit) markInstDataReturned(inst *wavefront.Inst, what string) {
	cu.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: cu.comp.State.InstTasks[inst.ID],
		Kind:   tasks.MilestoneKindData,
		What:   what,
	})
}

// hasInFlightVectorMemFor reports whether any vector-memory transaction for the
// given instruction is still in flight — including siblings parked in the
// shadow buffer during a flush resend but not yet re-sent.
func (cu *ComputeUnit) hasInFlightVectorMemFor(inst *wavefront.Inst) bool {
	for _, info := range cu.comp.State.Vector {
		if info.Inst == instID(inst) {
			return true
		}
	}

	for _, info := range cu.comp.State.ShadowVector {
		if info.Inst == instID(inst) {
			return true
		}
	}

	return false
}

// hasInFlightScalarMemFor reports whether any scalar-memory access for the given
// instruction is still in flight — including shadow-buffered siblings that have
// not been re-sent yet.
func (cu *ComputeUnit) hasInFlightScalarMemFor(inst *wavefront.Inst) bool {
	for _, info := range cu.comp.State.Scalar {
		if info.Inst == instID(inst) {
			return true
		}
	}

	for _, info := range cu.comp.State.ShadowScalar {
		if info.Inst == instID(inst) {
			return true
		}
	}

	return false
}

func (cu *ComputeUnit) processInputFromVectorMem() bool {
	madeProgress := false
	for i := 0; i < 16; i++ {
		rsp, ok := cu.vectorMemPort().RetrieveIncoming()
		if !ok {
			break
		}

		switch rsp.Payload.(type) {
		case memprotocol.DataReadyRsp:

			cu.handleVectorDataLoadReturn(rsp)
		case memprotocol.WriteDoneRsp:

			cu.handleVectorDataStoreRsp(rsp)
		default:

			log.Panicf("cannot handle rsp of type %s from vector mem port",
				reflect.TypeOf(rsp))
		}
		madeProgress = true
	}
	return madeProgress
}

//nolint:gocyclo
func (cu *ComputeUnit) handleVectorDataLoadReturn(
	rsp messaging.Msg,
) {
	matchIdx := -1
	for i, info := range cu.comp.State.Vector {
		if info.Read.ID != 0 && info.Read.ID == rsp.RspTo {
			matchIdx = i
			break
		}
	}
	if matchIdx < 0 {
		return
	}

	info := cu.comp.State.Vector[matchIdx]
	cu.comp.State.Vector = append(
		cu.comp.State.Vector[:matchIdx],
		cu.comp.State.Vector[matchIdx+1:]...)
	cu.completeRequest(info.Read.ID)

	wf := cu.wave(info.WF)
	inst := cu.instruction(info.Inst)

	for _, laneInfo := range info.Lanes {
		offset := laneInfo.Offset
		access := RegisterAccess{}
		access.WaveOffset = wf.VRegOffset
		access.Reg = insts.Regs[laneInfo.Register]
		access.RegCount = laneInfo.Count
		access.LaneID = laneInfo.Lane
		if inst.FormatType == insts.FLAT && inst.Opcode == 16 { // FLAT_LOAD_UBYTE
			access.Data = insts.Uint32ToBytes(uint32(rsp.Payload.(memprotocol.DataReadyRsp).Data[offset]))
		} else if inst.FormatType == insts.FLAT && inst.Opcode == 18 {
			access.Data = insts.Uint32ToBytes(uint32(rsp.Payload.(memprotocol.DataReadyRsp).Data[offset]))
		} else {
			end := offset + uint64(4*laneInfo.Count)
			if end > uint64(len(rsp.Payload.(memprotocol.DataReadyRsp).Data)) {
				end = uint64(len(rsp.Payload.(memprotocol.DataReadyRsp).Data))
				if offset >= end {
					continue
				}
			}
			access.Data = rsp.Payload.(memprotocol.DataReadyRsp).Data[offset:end]
		}
		cu.VRegFile[wf.SIMDID].Write(access)
	}

	// Coalesced responses can return out of order, so the last request
	// generated is not necessarily the last received. Release
	// the s_waitcnt counters, end the inst task and mark the data wait only
	// once no transaction for this instruction is still in flight; releasing
	// the counter earlier lets the wavefront reuse the destination VGPRs
	// before the remaining responses overwrite them.
	if !cu.hasInFlightVectorMemFor(cu.instruction(info.Inst)) {
		wf.OutstandingVectorMemAccess--
		if cu.instruction(info.Inst).FormatType == insts.FLAT {
			wf.OutstandingScalarMemAccess--
		}

		cu.markInstDataReturned(cu.instruction(info.Inst), "vmem")
		cu.logInstTask(wf, cu.instruction(info.Inst), true)
	}
}

func (cu *ComputeUnit) handleVectorDataStoreRsp(
	rsp messaging.Msg,
) {
	matchIdx := -1
	for i, info := range cu.comp.State.Vector {
		if info.Write.ID != 0 && info.Write.ID == rsp.RspTo {
			matchIdx = i
			break
		}
	}
	if matchIdx < 0 {
		return
	}

	info := cu.comp.State.Vector[matchIdx]
	cu.comp.State.Vector = append(
		cu.comp.State.Vector[:matchIdx],
		cu.comp.State.Vector[matchIdx+1:]...)
	cu.completeRequest(info.Write.ID)

	wf := cu.wave(info.WF)

	// Coalesced responses can return out of order; release the s_waitcnt
	// counters, end the inst task and mark the data wait only once no
	// transaction for this instruction remains in flight (see
	// handleVectorDataLoadReturn).
	if !cu.hasInFlightVectorMemFor(cu.instruction(info.Inst)) {
		wf.OutstandingVectorMemAccess--
		if cu.instruction(info.Inst).FormatType == insts.FLAT {
			wf.OutstandingScalarMemAccess--
		}

		cu.markInstDataReturned(cu.instruction(info.Inst), "vmem")
		cu.logInstTask(wf, cu.instruction(info.Inst), true)
	}
}

// UpdatePCAndSetReady is self explained
func (cu *ComputeUnit) UpdatePCAndSetReady(wf *wavefront.Wavefront) {
	wf.State = wavefront.WfReady
	wf.SetPC(wf.PC() + uint64(wf.Inst().ByteSize))
	cu.removeStaleInstBuffer(wf)
}

func (cu *ComputeUnit) removeStaleInstBuffer(wf *wavefront.Wavefront) {
	if len(wf.InstBuffer) != 0 {
		for wf.PC() >= wf.InstBufferStartPC+64 {
			wf.InstBuffer = wf.InstBuffer[64:]
			wf.InstBufferStartPC += 64
		}
	}
}

func wfHasInstructionReady(wf *wavefront.Wavefront) bool {
	if wf.NextInst() != nil {
		return true
	}

	if len(wf.InstBuffer) == 0 || wf.PC() < wf.InstBufferStartPC {
		return false
	}

	offset := wf.PC() - wf.InstBufferStartPC
	if offset > uint64(len(wf.InstBuffer)) {
		return false
	}

	return len(wf.InstBuffer[int(offset):]) >= 4
}

func (cu *ComputeUnit) flushCUBuffers() {
	cu.comp.State.InstFetch = nil
	cu.comp.State.Scalar = nil
	cu.comp.State.Vector = nil
}

// markWfMilestone records a milestone on the wavefront task itself (TaskID
// wf.UID), not on its instruction. These tile the wavefront's life into
// execution (work) and the reasons for a gap between instructions — waiting
// to fetch (hardware_resource), waiting for the fetched bytes (data), and
// waiting for the exec unit to accept the issue (hardware_resource). They are
// only emitted while nothing is in flight (see logInstTask), so they never
// overlap and a prefetch overlapping execution stays silent.
func (cu *ComputeUnit) markWfMilestone(
	wf *wavefront.Wavefront, kind tasks.MilestoneKind, what string,
) {
	cu.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: cu.comp.State.WFTasks[wf.UID],
		Kind:   kind,
		What:   what,
	})
}

func (cu *ComputeUnit) logInstTask(
	wf *wavefront.Wavefront,
	inst *wavefront.Inst,
	completed bool,
) {
	if completed {
		wf.InFlightInsts--
		if wf.InFlightInsts == 0 {
			// The last in-flight instruction finished: close the execution burst
			// as work, so the gap that follows is attributed separately.
			cu.markWfMilestone(wf, tasks.MilestoneKindWork, "execution")
		}
		cu.comp.TaskTracker().EndTask(cu.comp.State.InstTasks[inst.ID])
		delete(cu.comp.State.InstTasks, inst.ID)
		cu.releaseInstruction(inst.ID)
		return
	}

	if wf.InFlightInsts == 0 {
		// Issuing after a gap (nothing was in flight): the time since the last
		// milestone was spent waiting to issue this instruction — decode is ~1
		// cycle, so it is dominated by waiting for the exec unit to accept the
		// wave. Attribute it to that unit as a hardware resource.
		cu.markWfMilestone(wf, tasks.MilestoneKindHardwareResource,
			cu.comp.Name()+"."+cu.execUnitToString(inst.ExeUnit))
	}
	wf.InFlightInsts++

	cu.comp.State.InstTasks[inst.ID] = cu.comp.TaskTracker().StartTask(cu.comp, tasks.StartInfo{
		ParentID: cu.comp.State.WFTasks[wf.UID],
		Kind:     "inst",
		What:     cu.execUnitToString(inst.ExeUnit),
		Location: cu.comp.Name() + "." + cu.execUnitToString(inst.ExeUnit),
		Detail: map[string]interface{}{
			"inst": inst,
			"wf":   wf,
		},
	})
}

func (cu *ComputeUnit) execUnitToString(u insts.ExeUnit) string {
	switch u {
	case insts.ExeUnitVALU:
		return "VALU"
	case insts.ExeUnitScalar:
		return "Scalar"
	case insts.ExeUnitVMem:
		return "VMem"
	case insts.ExeUnitBranch:
		return "Branch"
	case insts.ExeUnitLDS:
		return "LDS"
	case insts.ExeUnitGDS:
		return "GDS"
	case insts.ExeUnitSpecial:
		return "Special"
	}
	panic("unknown exec unit")
}

func (cu *ComputeUnit) reInsertShadowBufferReqsToOriginalBuffers() bool {
	cu.comp.State.IsSendingOutShadowBufferReqs = false
	for i := 0; i < len(cu.comp.State.ShadowVector); i++ {
		cu.comp.State.Vector = append(
			cu.comp.State.Vector, cu.comp.State.ShadowVector[i])
	}

	for i := 0; i < len(cu.comp.State.ShadowScalar); i++ {
		cu.comp.State.Scalar = append(
			cu.comp.State.Scalar, cu.comp.State.ShadowScalar[i])
	}

	for i := 0; i < len(cu.comp.State.ShadowInstFetch); i++ {
		cu.comp.State.InstFetch = append(
			cu.comp.State.InstFetch, cu.comp.State.ShadowInstFetch[i])
	}

	return true
}

func (cu *ComputeUnit) checkShadowBuffers() bool {
	numReqsPendingToSend :=
		len(cu.comp.State.ShadowScalar) +
			len(cu.comp.State.ShadowVector) +
			len(cu.comp.State.ShadowInstFetch)

	if numReqsPendingToSend == 0 {
		cu.comp.State.IsSendingOutShadowBufferReqs = false
		cu.Scheduler.Resume()
		cu.comp.State.IsPaused = false
		return true
	}

	return cu.sendOutShadowBufferReqs()
}

func (cu *ComputeUnit) sendOutShadowBufferReqs() bool {
	madeProgress := false

	madeProgress = cu.sendScalarShadowBufferAccesses() || madeProgress
	madeProgress = cu.sendVectorShadowBufferAccesses() || madeProgress
	madeProgress = cu.sendInstFetchShadowBufferAccesses() || madeProgress

	return madeProgress
}

func (cu *ComputeUnit) sendScalarShadowBufferAccesses() bool {
	if len(cu.comp.State.ShadowScalar) > 0 {
		info := &cu.comp.State.ShadowScalar[0]

		info.Req.ID = cu.comp.NewID()
		if cu.scalarMemPort().CanSend() {
			cu.startRequest(info.Req, cu.comp.State.InstTasks[info.Inst])
			cu.scalarMemPort().Send(info.Req)
			// The resend has a fresh message ID; open a replacement req_out
			// task (the original was ended in populateShadowBuffers) so the
			// post-flush round trip is traced and completion matches.
			cu.comp.State.Scalar =
				append(cu.comp.State.Scalar, *info)
			cu.comp.State.ShadowScalar =
				cu.comp.State.ShadowScalar[1:]
			return true
		}
	}

	return false
}

func (cu *ComputeUnit) sendVectorShadowBufferAccesses() bool {
	if len(cu.comp.State.ShadowVector) > 0 {
		info := cu.comp.State.ShadowVector[0]
		if info.Read.ID != 0 {
			info.Read.ID = cu.comp.NewID()
			if cu.vectorMemPort().CanSend() {
				cu.startRequest(info.Read, cu.comp.State.InstTasks[info.Inst])
				cu.vectorMemPort().Send(info.Read)
				cu.comp.State.Vector = append(
					cu.comp.State.Vector, info)
				cu.comp.State.ShadowVector =
					cu.comp.State.ShadowVector[1:]
				return true
			}
		} else if info.Write.ID != 0 {
			info.Write.ID = cu.comp.NewID()
			if cu.vectorMemPort().CanSend() {
				cu.startRequest(info.Write, cu.comp.State.InstTasks[info.Inst])
				cu.vectorMemPort().Send(info.Write)
				cu.comp.State.Vector = append(
					cu.comp.State.Vector, info)
				cu.comp.State.ShadowVector =
					cu.comp.State.ShadowVector[1:]
				return true
			}
		}
	}
	return false
}

func (cu *ComputeUnit) sendInstFetchShadowBufferAccesses() bool {
	if len(cu.comp.State.ShadowInstFetch) > 0 {
		info := &cu.comp.State.ShadowInstFetch[0]
		info.Req.ID = cu.comp.NewID()
		if cu.instMemPort().CanSend() {
			cu.startRequest(info.Req, info.Task)
			cu.instMemPort().Send(info.Req)
			cu.comp.State.InstFetch = append(cu.comp.State.InstFetch, *info)
			cu.comp.State.ShadowInstFetch = cu.comp.State.ShadowInstFetch[1:]
			return true
		}
	}
	return false
}

func (cu *ComputeUnit) populateShadowBuffers() {
	// The shadowed accesses are re-sent after the restart with freshly
	// generated message IDs (see sendOutShadowBufferReqs), and the resend opens
	// a fresh req_out task for the new ID. End the original req_out tasks —
	// keyed on the old message ID — here, so they are not left unfinalized
	// across the flush. EndTaskOnReset is a no-op for an access that was
	// buffered but never sent (no req_out was opened).
	for i := 0; i < len(cu.comp.State.InstFetch); i++ {
		cu.completeRequest(cu.comp.State.InstFetch[i].Req.ID)
		cu.comp.State.ShadowInstFetch = append(
			cu.comp.State.ShadowInstFetch, cu.comp.State.InstFetch[i])
	}

	for i := 0; i < len(cu.comp.State.Scalar); i++ {
		cu.completeRequest(cu.comp.State.Scalar[i].Req.ID)
		cu.comp.State.ShadowScalar = append(
			cu.comp.State.ShadowScalar, cu.comp.State.Scalar[i])
	}

	for i := 0; i < len(cu.comp.State.Vector); i++ {
		info := cu.comp.State.Vector[i]
		if info.Read.ID != 0 {
			cu.completeRequest(info.Read.ID)
		}
		if info.Write.ID != 0 {
			cu.completeRequest(info.Write.ID)
		}
		cu.comp.State.ShadowVector = append(
			cu.comp.State.ShadowVector, info)
	}

	cu.comp.State.Scalar = nil
	cu.comp.State.InstFetch = nil
	cu.comp.State.Vector = nil
}

func (cu *ComputeUnit) setWavesToReady() {
	for _, wfPool := range cu.WfPools {
		for _, wf := range wfPool.waves() {
			if wf.State != wavefront.WfCompleted {
				wf.State = wavefront.WfReady
				wf.IsFetching = false
			}
		}
	}
}

func (cu *ComputeUnit) wavefrontByID(id uint64) *wavefront.Wavefront {
	wf := cu.findWavefront(id)
	if wf == nil {
		panic("cu: unknown wavefront ID")
	}
	return wf
}

func (cu *ComputeUnit) findWavefront(id uint64) *wavefront.Wavefront {
	cu.ensureState()
	return cu.waves[id]
}

func (cu *ComputeUnit) startRequest(req messaging.Msg, parent uint64) {
	cu.comp.State.RequestTasks[req.ID] = cu.comp.TaskTracker().StartReqOut(cu.comp, req, parent)
}
func (cu *ComputeUnit) completeRequest(id uint64) {
	cu.comp.TaskTracker().EndReqOut(cu.comp.State.RequestTasks[id])
	delete(cu.comp.State.RequestTasks, id)
}
func (cu *ComputeUnit) completeWGTask(id uint64) {
	cu.comp.TaskTracker().EndReqIn(cu.comp.State.WGTasks[id])
	delete(cu.comp.State.WGTasks, id)
}
