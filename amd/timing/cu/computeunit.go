package cu

import (
	"log"
	"reflect"
	"slices"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
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
// component: its Tick method completes sampled waves, advances the pipeline,
// and drains completed groups.
//
// Mutable execution values belong to comp.State; the middleware holds only
// behavior, fixed dependencies, and a pure immutable instruction-description cache.
type ComputeUnit struct {
	comp *Comp

	WfDispatcher WfDispatcher
	Decoder      emu.Decoder
	decoded      map[decodeKey]*insts.Inst
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

// CurrentTime returns the current simulation time.
func (cu *ComputeUnit) CurrentTime() timing.VTimeInPicoSec {
	return cu.comp.CurrentTime()
}

// Tick advances the pipeline, handles input, and releases drained groups.
func (cu *ComputeUnit) Tick() bool {
	madeProgress := cu.completeSampledWaves()
	if madeProgress {
		cu.retireGroups()
	}

	madeProgress = cu.runPipeline() || madeProgress
	madeProgress = cu.processInput() || madeProgress
	// A zero-duration prediction added by processInput is due on this edge.
	// TickAt deduplicates the current edge, so finish it before leaving the tick.
	madeProgress = cu.completeSampledWaves() || madeProgress
	cu.retireGroups()

	return madeProgress
}

//nolint:gocyclo
func (cu *ComputeUnit) runPipeline() bool {
	madeProgress := false

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

	return madeProgress
}

func (cu *ComputeUnit) processInput() bool {
	madeProgress := false

	madeProgress = cu.processInputFromACE() || madeProgress
	madeProgress = cu.processInputFromInstMem() || madeProgress
	madeProgress = cu.processInputFromScalarMem() || madeProgress
	madeProgress = cu.processInputFromVectorMem() || madeProgress

	return madeProgress
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

// completeSampledWaves drains the time-ordered State list under the tick's lock.
func (cu *ComputeUnit) completeSampledWaves() bool {
	pending := cu.comp.State.SampledCompletions
	n := 0
	for n < len(pending) && pending[n].At <= cu.CurrentTime() {
		wf := cu.wave(pending[n].Wave)
		wf.state().StopFetch = true
		wf.execution().State = wavefront.WfCompleted
		// Sampled waves never initialize or use physical register locations.
		cu.endWaveTask(wf)
		n++
	}
	cu.comp.State.SampledCompletions = slices.Delete(pending, 0, n)
	return n > 0
}

func (cu *ComputeUnit) scheduleSampledCompletion(at timing.VTimeInPicoSec, wave wavefront.Handle) {
	pending := cu.comp.State.SampledCompletions
	// Insert after equal-time entries to preserve wave dispatch order.
	i := len(pending)
	for i > 0 && pending[i-1].At > at {
		i--
	}
	cu.comp.State.SampledCompletions = slices.Insert(pending, i, SampledCompletion{At: at, Wave: wave})
	cu.comp.TickAt(at)
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

	cu.comp.State.WGTasks[req.ID] = cu.comp.TaskTracker().StartReqIn(cu.comp, req)

	skipSimulate := cu.scheduleSampledWaves(wg, now)

	if !skipSimulate {
		for i, wf := range wg.waves() {
			location := req.Payload.(protocol.MapWGReq).Wavefronts[i]
			cu.WfPools[location.SIMDID].AddWf(wf)
			cu.WfDispatcher.DispatchWf(wf, req.Payload.(protocol.MapWGReq).Wavefronts[i])
			wf.execution().State = wavefront.WfReady

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

func (cu *ComputeUnit) scheduleSampledWaves(wg *WorkGroup, now timing.VTimeInPicoSec) bool {
	// sampling
	skipSimulate := false
	if cu.comp.Resources.Sampler != nil && !cu.comp.Resources.Sampler.IfDisable() {
		for _, wf := range wg.waves() {
			cu.comp.State.WFTimes[wf.UID] = now
		}

		wfpredicttime, wfsampled := cu.comp.Resources.Sampler.Predict()
		predtime := wfpredicttime
		skipSimulate = wfsampled

		for _, wf := range wg.waves() {
			if skipSimulate {
				predictedTime := predtime + now
				wf.execution().State = wavefront.WfSampledWaiting
				cu.scheduleSampledCompletion(predictedTime, handleOf(wf))
				cu.comp.State.WFTasks[wf.UID] = cu.comp.TaskTracker().StartTask(cu.comp, tasks.StartInfo{
					ParentID: cu.comp.State.WGTasks[wg.state().Request.ID],
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

func (cu *ComputeUnit) clearWGResource(wg *WorkGroup) {
	g := wg.state()
	belongsToGroup := func(id waveHandle) bool { return slices.Contains(g.Waves, id) }
	cu.comp.State.Scheduler.Internal = slices.DeleteFunc(cu.comp.State.Scheduler.Internal, belongsToGroup)
	cu.comp.State.Scheduler.Barrier = slices.DeleteFunc(cu.comp.State.Scheduler.Barrier, belongsToGroup)
	for _, wf := range wg.waves() {
		wfPool := cu.WfPools[wf.execution().SIMDID]
		wfPool.RemoveWf(wf)
	}
}

func (cu *ComputeUnit) isAllWfInWGCompleted(wg *WorkGroup) bool {
	for _, wf := range wg.waves() {
		if wf.execution().State != wavefront.WfCompleted {
			return false
		}
	}
	return true
}

func (cu *ComputeUnit) hasMoreWfsToRun() bool {
	for _, wfpool := range cu.WfPools {
		if len(wfpool.ids()) > 0 {
			return true
		}
	}
	return false
}

func (cu *ComputeUnit) wrapWG(
	raw *kernels.WorkGroup,
	req messaging.Msg,
) *WorkGroup {
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
	wasWaitingForFetch := wf.execution().InFlightInsts == 0 && !wfHasInstructionReady(wf)
	cu.comp.State.InstFetch = append(
		cu.comp.State.InstFetch[:matchIdx],
		cu.comp.State.InstFetch[matchIdx+1:]...)

	if addr == wf.execution().InstBufferStartPC+uint64(len(wf.execution().InstBuffer)) {
		wf.execution().InstBuffer = append(wf.execution().InstBuffer, rsp.Payload.(memprotocol.DataReadyRsp).Data...)
	}

	wf.execution().IsFetching = false
	if wf.execution().State == wavefront.WfDraining {
		wf.execution().State = wavefront.WfCompleted
	}
	wf.execution().LastFetchTime = cu.CurrentTime()

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
		WaveOffset: wf.execution().SRegOffset,
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
	if !cu.hasInFlightScalarMemFor(info.Inst) {
		wf.execution().OutstandingScalarMemAccess--
		cu.markInstDataReturned(info.Inst, "smem")
		cu.completeInstruction(wf, info.Inst)
	}
}

// markInstDataReturned records a "data" milestone on a memory instruction's
// task when its last memory response returns. The instruction's task ends at
// that same moment, so this closes the otherwise-unattributed gap between the
// memory request going out (or the in-flight slot being acquired) and the data
// coming back.
func (cu *ComputeUnit) markInstDataReturned(inst uint64, what string) {
	cu.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: cu.comp.State.InstTasks[inst],
		Kind:   tasks.MilestoneKindData,
		What:   what,
	})
}

// hasInFlightVectorMemFor reports whether any vector-memory transaction for the
// given instruction is still in flight.
func (cu *ComputeUnit) hasInFlightVectorMemFor(inst uint64) bool {
	for _, info := range cu.comp.State.Vector {
		if info.Inst == inst {
			return true
		}
	}

	return false
}

// hasInFlightScalarMemFor reports whether any scalar-memory access for the given
// instruction is still in flight.
func (cu *ComputeUnit) hasInFlightScalarMemFor(inst uint64) bool {
	for _, info := range cu.comp.State.Scalar {
		if info.Inst == inst {
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
	matchIdx := cu.vectorTransactionIndex(rsp.RspTo)
	if matchIdx < 0 || cu.comp.State.Vector[matchIdx].Read.ID == 0 {
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
		access.WaveOffset = wf.execution().VRegOffset
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
		cu.VRegFile[wf.execution().SIMDID].Write(access)
	}

	// Coalesced responses can return out of order, so the last request
	// generated is not necessarily the last received. Release
	// the s_waitcnt counters, end the inst task and mark the data wait only
	// once no transaction for this instruction is still in flight; releasing
	// the counter earlier lets the wavefront reuse the destination VGPRs
	// before the remaining responses overwrite them.
	if !cu.hasInFlightVectorMemFor(info.Inst) {
		wf.execution().OutstandingVectorMemAccess--
		if cu.instruction(info.Inst).FormatType == insts.FLAT {
			wf.execution().OutstandingScalarMemAccess--
		}

		cu.markInstDataReturned(info.Inst, "vmem")
		cu.completeInstruction(wf, info.Inst)
	}
}

func (cu *ComputeUnit) handleVectorDataStoreRsp(
	rsp messaging.Msg,
) {
	matchIdx := cu.vectorTransactionIndex(rsp.RspTo)
	if matchIdx < 0 || cu.comp.State.Vector[matchIdx].Write.ID == 0 {
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
	if !cu.hasInFlightVectorMemFor(info.Inst) {
		wf.execution().OutstandingVectorMemAccess--
		if cu.instruction(info.Inst).FormatType == insts.FLAT {
			wf.execution().OutstandingScalarMemAccess--
		}

		cu.markInstDataReturned(info.Inst, "vmem")
		cu.completeInstruction(wf, info.Inst)
	}
}

// UpdatePCAndSetReady is self explained
func (cu *ComputeUnit) UpdatePCAndSetReady(wf *Wavefront) {
	wf.execution().State = wavefront.WfReady
	wf.SetPC(wf.PC() + uint64(wf.Inst().ByteSize))
	cu.removeStaleInstBuffer(wf)
}

func (cu *ComputeUnit) removeStaleInstBuffer(wf *Wavefront) {
	if len(wf.execution().InstBuffer) != 0 {
		for wf.PC() >= wf.execution().InstBufferStartPC+64 {
			wf.execution().InstBuffer = wf.execution().InstBuffer[64:]
			wf.execution().InstBufferStartPC += 64
		}
	}
}

func wfHasInstructionReady(wf *Wavefront) bool {
	if wf.NextInst() != nil {
		return true
	}

	if len(wf.execution().InstBuffer) == 0 || wf.PC() < wf.execution().InstBufferStartPC {
		return false
	}

	offset := wf.PC() - wf.execution().InstBufferStartPC
	if offset > uint64(len(wf.execution().InstBuffer)) {
		return false
	}

	return len(wf.execution().InstBuffer[int(offset):]) >= 4
}

// markWfMilestone records a milestone on the wavefront task itself (TaskID
// wf.UID), not on its instruction. These tile the wavefront's life into
// execution (work) and the reasons for a gap between instructions — waiting
// to fetch (hardware_resource), waiting for the fetched bytes (data), and
// waiting for the exec unit to accept the issue (hardware_resource). They are
// only emitted while nothing is in flight (see beginInstruction), so they never
// overlap and a prefetch overlapping execution stays silent.
func (cu *ComputeUnit) markWfMilestone(
	wf *Wavefront, kind tasks.MilestoneKind, what string,
) {
	cu.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: cu.comp.State.WFTasks[wf.UID],
		Kind:   kind,
		What:   what,
	})
}

// completeInstruction ends execution (including every async memory response).
// Observers can still resolve the instruction until their completion callbacks return.
func (cu *ComputeUnit) completeInstruction(wf *Wavefront, inst uint64) {
	wf.execution().InFlightInsts--
	if wf.execution().InFlightInsts == 0 {
		// The last in-flight instruction finished: close the execution burst
		// as work, so the gap that follows is attributed separately.
		cu.markWfMilestone(wf, tasks.MilestoneKindWork, "execution")
	}
	cu.comp.TaskTracker().EndTask(cu.comp.State.InstTasks[inst])
	delete(cu.comp.State.InstTasks, inst)
	instruction := cu.comp.State.Instructions[inst]
	instruction.ExecutionPending = false
	cu.comp.State.Instructions[inst] = instruction
	cu.releaseInstruction(inst)
}

// beginInstruction starts semantic execution even when task tracking is disabled.
func (cu *ComputeUnit) beginInstruction(wf *Wavefront, inst uint64) {
	description := cu.instruction(inst)
	instruction := cu.comp.State.Instructions[inst]
	instruction.ExecutionPending = true
	cu.comp.State.Instructions[inst] = instruction

	if wf.execution().InFlightInsts == 0 {
		// Issuing after a gap (nothing was in flight): the time since the last
		// milestone was spent waiting to issue this instruction — decode is ~1
		// cycle, so it is dominated by waiting for the exec unit to accept the
		// wave. Attribute it to that unit as a hardware resource.
		cu.markWfMilestone(wf, tasks.MilestoneKindHardwareResource,
			cu.comp.Name()+"."+cu.execUnitToString(description.ExeUnit))
	}
	wf.execution().InFlightInsts++

	cu.comp.State.InstTasks[inst] = cu.comp.TaskTracker().StartTask(cu.comp, tasks.StartInfo{
		ParentID: cu.comp.State.WFTasks[wf.UID],
		Kind:     "inst",
		What:     cu.execUnitToString(description.ExeUnit),
		Location: cu.comp.Name() + "." + cu.execUnitToString(description.ExeUnit),
		Detail: map[string]interface{}{
			"inst":   inst,
			"format": description.FormatName,
			"wf":     wf.UID,
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

func (cu *ComputeUnit) wavefrontByID(id uint64) *Wavefront {
	wf := cu.findWavefront(id)
	if wf == nil {
		panic("cu: unknown wavefront ID")
	}
	return wf
}

func (cu *ComputeUnit) findWavefront(id uint64) *Wavefront {
	// ID lookup serves the ISA debugger; execution uses fixed-slot handles.
	for index := range cu.comp.State.Waves {
		state := &cu.comp.State.Waves[index]
		if state.ID == id && id != 0 {
			return &Wavefront{cu: cu, UID: id, handle: makeWaveHandle(index, state.Generation)}
		}
	}
	return nil
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

func (cu *ComputeUnit) endWaveTask(wf *Wavefront) {
	if task, ok := cu.comp.State.WFTasks[wf.UID]; ok {
		cu.comp.TaskTracker().EndTask(task)
		delete(cu.comp.State.WFTasks, wf.UID)
	}
}

func (cu *ComputeUnit) sendWGCompletionMessage(
	wg *WorkGroup,
) (done bool) {
	if !cu.acePort().CanSend() {
		return false
	}
	mapReq := wg.state().Request
	dispatcher := mapReq.Src

	msg := messaging.Msg{ID: cu.comp.NewID(),
		Src: cu.acePort().AsRemote(),
		Dst: dispatcher,
		Payload: protocol.WGCompletionMsg{RspToIDs: []uint64{mapReq.ID},
			Timings: workGroupTimings(wg)}}

	cu.acePort().Send(msg)

	return true
}
