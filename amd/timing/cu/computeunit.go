package cu

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/sampling"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// A ComputeUnit provides a detailed and accurate simulation of a GCN3
// ComputeUnit. It is the (single) middleware of the timing compute-unit
// component: its Tick method reproduces the v4 ComputeUnit.Tick body, and it
// also serves as the timing.Handler for the CU's custom events
// (WfCompletionEvent, WfDispatchEvent).
//
// TODO(akita5): state purity — the fields below (wavefront pools, in-flight
// access records, sub-units, register files) hold pointers and cannot live in
// the component State yet.
type ComputeUnit struct {
	comp   *Comp
	engine timing.EventScheduler

	// Handler IDs that the custom events of this CU dispatch on.
	wfCompletionHandlerID string
	wfDispatchHandlerID   string

	WfDispatcher WfDispatcher
	Decoder      emu.Decoder
	WfPools      []*WavefrontPool

	InFlightInstFetch            []*InstFetchReqInfo
	InFlightScalarMemAccess      []*ScalarMemAccessInfo
	InFlightVectorMemAccess      []VectorMemAccessInfo
	InFlightVectorMemAccessLimit int

	shadowInFlightInstFetch       []*InstFetchReqInfo
	shadowInFlightScalarMemAccess []*ScalarMemAccessInfo
	shadowInFlightVectorMemAccess []VectorMemAccessInfo

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

	// wftime records, for sampling, the time each wavefront was mapped.
	wftime map[uint64]timing.VTimeInPicoSec
}

// Comp returns the component this middleware belongs to.
func (cu *ComputeUnit) Comp() *Comp {
	return cu.comp
}

func (cu *ComputeUnit) acePort() messaging.Port {
	if cu.ToACE == nil {
		cu.ToACE = cu.comp.GetPortByName(DispatchPortName)
	}

	return cu.ToACE
}

func (cu *ComputeUnit) instMemPort() messaging.Port {
	if cu.ToInstMem == nil {
		cu.ToInstMem = cu.comp.GetPortByName(InstMemPortName)
	}

	return cu.ToInstMem
}

func (cu *ComputeUnit) scalarMemPort() messaging.Port {
	if cu.ToScalarMem == nil {
		cu.ToScalarMem = cu.comp.GetPortByName(ScalarMemPortName)
	}

	return cu.ToScalarMem
}

func (cu *ComputeUnit) vectorMemPort() messaging.Port {
	if cu.ToVectorMem == nil {
		cu.ToVectorMem = cu.comp.GetPortByName(VectorMemPortName)
	}

	return cu.ToVectorMem
}

func (cu *ComputeUnit) cpPort() messaging.Port {
	if cu.ToCP == nil {
		cu.ToCP = cu.comp.GetPortByName(CtrlPortName)
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
	madeProgress := false

	madeProgress = cu.runPipeline() || madeProgress
	madeProgress = cu.sendToCP() || madeProgress
	madeProgress = cu.processInput() || madeProgress
	madeProgress = cu.doFlush() || madeProgress

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
	req := cu.cpPort().PeekIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case protocol.CUPipelineRestartReq:
		return cu.handlePipelineResume(req)
	case protocol.CUPipelineFlushReq:
		return cu.handlePipelineFlushReq(req)
	default:
		panic("unknown msg type")
	}
}

func (cu *ComputeUnit) handlePipelineFlushReq(
	req protocol.CUPipelineFlushReq,
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
	req protocol.CUPipelineRestartReq,
) bool {
	// v4 sent the restart response unconditionally and panicked when the
	// port was full. v5 ports panic on Send when full, so the request stays
	// queued until the response can go out.
	if !cu.cpPort().CanSend() {
		return false
	}

	state := &cu.comp.State
	state.IsSendingOutShadowBufferReqs = true

	rsp := protocol.CUPipelineRestartRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			Src:   cu.cpPort().AsRemote(),
			Dst:   req.Src,
			RspTo: req.ID,
		},
	}
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

	rsp := protocol.CUPipelineFlushRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			Src:   cu.cpPort().AsRemote(),
			Dst:   state.PendingCPRspDst,
			RspTo: state.PendingCPRspTo,
		},
	}
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

	cu.shadowInFlightInstFetch = nil
	cu.shadowInFlightScalarMemAccess = nil
	cu.shadowInFlightVectorMemAccess = nil

	cu.populateShadowBuffers()
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

func (cu *ComputeUnit) processInputFromACE() bool {
	req := cu.acePort().RetrieveIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case protocol.MapWGReq:
		return cu.handleMapWGReq(req)
	default:
		panic("unknown req type")
	}
}

// Handle processes the custom events of the compute unit (the regular tick
// events are dispatched to the component's own handler).
func (cu *ComputeUnit) Handle(evt timing.Event) error {
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

	return nil
}

func (cu *ComputeUnit) handleWfCompletionEvent(
	evt wavefront.WfCompletionEvent,
) {
	wf := evt.Wf
	wf.State = wavefront.WfCompleted
	s := cu.Scheduler.(*SchedulerImpl)

	if !s.areAllOtherWfsInWGCompleted(wf.WG, wf) {
		return
	}

	now := evt.Time()

	done := s.sendWGCompletionMessage(wf.WG)
	if !done {
		newEvent := wavefront.NewWfCompletionEvent(
			cu.comp.Spec().Freq.NextTick(now), cu.wfCompletionHandlerID, wf)
		cu.engine.Schedule(newEvent)

		return
	}

	s.resetRegisterValue(wf)
	cu.clearWGResource(wf.WG)
	tracing.EndTask(cu.comp, tracing.TaskEnd{ID: wf.UID})
	tracing.TraceReqComplete(cu.comp, wf.WG.MapReq)
}

func (cu *ComputeUnit) handleMapWGReq(
	req protocol.MapWGReq,
) bool {
	now := cu.CurrentTime()

	wg := cu.wrapWG(req.WorkGroup, req)

	tracing.TraceReqReceive(cu.comp, req)

	// sampling
	skipSimulate := false
	if *sampling.SampledRunnerFlag {
		for _, wf := range wg.Wfs {
			cu.wftime[wf.UID] = now
		}

		wfpredicttime, wfsampled := sampling.SampledEngineInstance.Predict()
		predtime := wfpredicttime
		skipSimulate = wfsampled

		for _, wf := range wg.Wfs {
			if skipSimulate {
				predictedTime := predtime + now
				wf.State = wavefront.WfSampledCompleted
				newEvent := wavefront.NewWfCompletionEvent(
					predictedTime, cu.wfCompletionHandlerID, wf)
				cu.engine.Schedule(newEvent)
				tracing.StartTask(cu.comp, tracing.TaskStart{
					ID:       wf.UID,
					ParentID: tracing.MsgIDAtReceiver(req, cu.comp),
					Kind:     "wavefront",
					What:     "wavefront",
				})
			}
		}
	}

	if !skipSimulate {
		for i, wf := range wg.Wfs {
			location := req.Wavefronts[i]
			cu.WfPools[location.SIMDID].AddWf(wf)
			cu.WfDispatcher.DispatchWf(wf, req.Wavefronts[i])
			wf.State = wavefront.WfReady

			tracing.StartTask(cu.comp, tracing.TaskStart{
				ID:       wf.UID,
				ParentID: tracing.MsgIDAtReceiver(req, cu.comp),
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

func (cu *ComputeUnit) clearWGResource(wg *wavefront.WorkGroup) {
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
		if len(wfpool.wfs) > 0 {
			return true
		}
	}
	return false
}

func (cu *ComputeUnit) wrapWG(
	raw *kernels.WorkGroup,
	req protocol.MapWGReq,
) *wavefront.WorkGroup {
	wg := wavefront.NewWorkGroup(raw, req)

	lds := make([]byte, req.WorkGroup.Packet.GroupSegmentSize)
	wg.LDS = lds

	for _, rawWf := range req.WorkGroup.Wavefronts {
		wf := wavefront.NewWavefront(rawWf)
		wf.RegAccessor = &CURegFileAccessor{CU: cu, WF: wf}
		wg.Wfs = append(wg.Wfs, wf)
		wf.WG = wg
		wf.SetPID(req.PID)
	}

	return wg
}

func (cu *ComputeUnit) processInputFromInstMem() bool {
	rsp := cu.instMemPort().RetrieveIncoming()
	if rsp == nil {
		return false
	}

	switch rsp := rsp.(type) {
	case memprotocol.DataReadyRsp:
		cu.handleFetchReturn(rsp)
	default:
		log.Panicf("cannot handle request of type %s from InstMem port",
			reflect.TypeOf(rsp))
	}
	return true
}

func (cu *ComputeUnit) handleFetchReturn(
	rsp memprotocol.DataReadyRsp,
) bool {
	matchIdx := -1
	for i, info := range cu.InFlightInstFetch {
		if info.Req.ID == rsp.RspTo {
			matchIdx = i
			break
		}
	}
	if matchIdx < 0 {
		return false
	}

	info := cu.InFlightInstFetch[matchIdx]
	wf := info.Wavefront
	addr := info.Address
	cu.InFlightInstFetch = append(
		cu.InFlightInstFetch[:matchIdx],
		cu.InFlightInstFetch[matchIdx+1:]...)

	if addr == wf.InstBufferStartPC+uint64(len(wf.InstBuffer)) {
		wf.InstBuffer = append(wf.InstBuffer, rsp.Data...)
	}

	wf.IsFetching = false
	wf.LastFetchTime = cu.CurrentTime()

	tracing.TraceReqFinalize(cu.comp, info.Req)
	tracing.EndTask(cu.comp, tracing.TaskEnd{ID: info.FetchTaskID})
	return true
}

func (cu *ComputeUnit) processInputFromScalarMem() bool {
	rsp := cu.scalarMemPort().RetrieveIncoming()
	if rsp == nil {
		return false
	}

	switch rsp := rsp.(type) {
	case memprotocol.DataReadyRsp:
		cu.handleScalarDataLoadReturn(rsp)
	default:
		log.Panicf("cannot handle request of type %s from ScalarMem port",
			reflect.TypeOf(rsp))
	}
	return true
}

func (cu *ComputeUnit) handleScalarDataLoadReturn(
	rsp memprotocol.DataReadyRsp,
) {
	matchIdx := -1
	for i, info := range cu.InFlightScalarMemAccess {
		if info.Req.ID == rsp.RspTo {
			matchIdx = i
			break
		}
	}
	if matchIdx < 0 {
		return
	}

	info := cu.InFlightScalarMemAccess[matchIdx]
	req := info.Req

	wf := info.Wavefront
	access := RegisterAccess{
		WaveOffset: wf.SRegOffset,
		Reg:        info.DstSGPR,
		RegCount:   len(rsp.Data) / 4,
		Data:       rsp.Data,
	}
	cu.SRegFile.Write(access)

	cu.InFlightScalarMemAccess = append(
		cu.InFlightScalarMemAccess[:matchIdx],
		cu.InFlightScalarMemAccess[matchIdx+1:]...)

	tracing.TraceReqFinalize(cu.comp, req)

	if cu.isLastRead(req) {
		wf.OutstandingScalarMemAccess--
		cu.logInstTask(wf, info.Inst, true)
	}
}

func (cu *ComputeUnit) isLastRead(req memprotocol.ReadReq) bool {
	return !req.CanWaitForCoalesce
}

func (cu *ComputeUnit) processInputFromVectorMem() bool {
	madeProgress := false
	for i := 0; i < 16; i++ {
		rsp := cu.vectorMemPort().RetrieveIncoming()
		if rsp == nil {
			break
		}

		switch rsp := rsp.(type) {
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
	rsp memprotocol.DataReadyRsp,
) {
	matchIdx := -1
	for i, info := range cu.InFlightVectorMemAccess {
		if info.Read != nil && info.Read.ID == rsp.RspTo {
			matchIdx = i
			break
		}
	}
	if matchIdx < 0 {
		return
	}

	info := cu.InFlightVectorMemAccess[matchIdx]
	cu.InFlightVectorMemAccess = append(
		cu.InFlightVectorMemAccess[:matchIdx],
		cu.InFlightVectorMemAccess[matchIdx+1:]...)
	tracing.TraceReqFinalize(cu.comp, *info.Read)

	wf := info.Wavefront
	inst := info.Inst

	for _, laneInfo := range info.laneInfo {
		offset := laneInfo.addrOffsetInCacheLine
		access := RegisterAccess{}
		access.WaveOffset = wf.VRegOffset
		access.Reg = laneInfo.reg
		access.RegCount = laneInfo.regCount
		access.LaneID = laneInfo.laneID
		if inst.FormatType == insts.FLAT && inst.Opcode == 16 { // FLAT_LOAD_UBYTE
			access.Data = insts.Uint32ToBytes(uint32(rsp.Data[offset]))
		} else if inst.FormatType == insts.FLAT && inst.Opcode == 18 {
			access.Data = insts.Uint32ToBytes(uint32(rsp.Data[offset]))
		} else {
			end := offset + uint64(4*laneInfo.regCount)
			if end > uint64(len(rsp.Data)) {
				end = uint64(len(rsp.Data))
				if offset >= end {
					continue
				}
			}
			access.Data = rsp.Data[offset:end]
		}
		cu.VRegFile[wf.SIMDID].Write(access)
	}

	if !info.Read.CanWaitForCoalesce {
		wf.OutstandingVectorMemAccess--
		if info.Inst.FormatType == insts.FLAT {
			wf.OutstandingScalarMemAccess--
		}

		cu.logInstTask(wf, info.Inst, true)
	}
}

func (cu *ComputeUnit) handleVectorDataStoreRsp(
	rsp memprotocol.WriteDoneRsp,
) {
	matchIdx := -1
	for i, info := range cu.InFlightVectorMemAccess {
		if info.Write != nil && info.Write.ID == rsp.RspTo {
			matchIdx = i
			break
		}
	}
	if matchIdx < 0 {
		return
	}

	info := cu.InFlightVectorMemAccess[matchIdx]
	cu.InFlightVectorMemAccess = append(
		cu.InFlightVectorMemAccess[:matchIdx],
		cu.InFlightVectorMemAccess[matchIdx+1:]...)
	tracing.TraceReqFinalize(cu.comp, *info.Write)

	wf := info.Wavefront
	if !info.Write.CanWaitForCoalesce {
		wf.OutstandingVectorMemAccess--
		if info.Inst.FormatType == insts.FLAT {
			wf.OutstandingScalarMemAccess--
		}
		cu.logInstTask(wf, info.Inst, true)
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

func (cu *ComputeUnit) flushCUBuffers() {
	cu.InFlightInstFetch = nil
	cu.InFlightScalarMemAccess = nil
	cu.InFlightVectorMemAccess = nil
}

func (cu *ComputeUnit) logInstTask(
	wf *wavefront.Wavefront,
	inst *wavefront.Inst,
	completed bool,
) {
	if completed {
		tracing.EndTask(cu.comp, tracing.TaskEnd{ID: inst.ID})
		return
	}

	tracing.StartTask(cu.comp, tracing.TaskStart{
		ID:       inst.ID,
		ParentID: wf.UID,
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
	for i := 0; i < len(cu.shadowInFlightVectorMemAccess); i++ {
		cu.InFlightVectorMemAccess = append(
			cu.InFlightVectorMemAccess, cu.shadowInFlightVectorMemAccess[i])
	}

	for i := 0; i < len(cu.shadowInFlightScalarMemAccess); i++ {
		cu.InFlightScalarMemAccess = append(
			cu.InFlightScalarMemAccess, cu.shadowInFlightScalarMemAccess[i])
	}

	for i := 0; i < len(cu.shadowInFlightInstFetch); i++ {
		cu.InFlightInstFetch = append(
			cu.InFlightInstFetch, cu.shadowInFlightInstFetch[i])
	}

	return true
}

func (cu *ComputeUnit) checkShadowBuffers() bool {
	numReqsPendingToSend :=
		len(cu.shadowInFlightScalarMemAccess) +
			len(cu.shadowInFlightVectorMemAccess) +
			len(cu.shadowInFlightInstFetch)

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
	if len(cu.shadowInFlightScalarMemAccess) > 0 {
		info := cu.shadowInFlightScalarMemAccess[0]

		info.Req.ID = timing.GetIDGenerator().Generate()
		if cu.scalarMemPort().CanSend() {
			cu.scalarMemPort().Send(info.Req)
			cu.InFlightScalarMemAccess =
				append(cu.InFlightScalarMemAccess, info)
			cu.shadowInFlightScalarMemAccess =
				cu.shadowInFlightScalarMemAccess[1:]
			return true
		}
	}

	return false
}

func (cu *ComputeUnit) sendVectorShadowBufferAccesses() bool {
	if len(cu.shadowInFlightVectorMemAccess) > 0 {
		info := cu.shadowInFlightVectorMemAccess[0]
		if info.Read != nil {
			info.Read.ID = timing.GetIDGenerator().Generate()
			if cu.vectorMemPort().CanSend() {
				cu.vectorMemPort().Send(*info.Read)
				cu.InFlightVectorMemAccess = append(
					cu.InFlightVectorMemAccess, info)
				cu.shadowInFlightVectorMemAccess =
					cu.shadowInFlightVectorMemAccess[1:]
				return true
			}
		} else if info.Write != nil {
			info.Write.ID = timing.GetIDGenerator().Generate()
			if cu.vectorMemPort().CanSend() {
				cu.vectorMemPort().Send(*info.Write)
				cu.InFlightVectorMemAccess = append(
					cu.InFlightVectorMemAccess, info)
				cu.shadowInFlightVectorMemAccess =
					cu.shadowInFlightVectorMemAccess[1:]
				return true
			}
		}
	}
	return false
}

func (cu *ComputeUnit) sendInstFetchShadowBufferAccesses() bool {
	if len(cu.shadowInFlightInstFetch) > 0 {
		info := cu.shadowInFlightInstFetch[0]
		info.Req.ID = timing.GetIDGenerator().Generate()
		if cu.instMemPort().CanSend() {
			cu.instMemPort().Send(info.Req)
			cu.InFlightInstFetch = append(cu.InFlightInstFetch, info)
			cu.shadowInFlightInstFetch = cu.shadowInFlightInstFetch[1:]
			return true
		}
	}
	return false
}

func (cu *ComputeUnit) populateShadowBuffers() {
	for i := 0; i < len(cu.InFlightInstFetch); i++ {
		cu.shadowInFlightInstFetch = append(
			cu.shadowInFlightInstFetch, cu.InFlightInstFetch[i])
	}

	for i := 0; i < len(cu.InFlightScalarMemAccess); i++ {
		cu.shadowInFlightScalarMemAccess = append(
			cu.shadowInFlightScalarMemAccess, cu.InFlightScalarMemAccess[i])
	}

	for i := 0; i < len(cu.InFlightVectorMemAccess); i++ {
		cu.shadowInFlightVectorMemAccess = append(
			cu.shadowInFlightVectorMemAccess, cu.InFlightVectorMemAccess[i])
	}

	cu.InFlightScalarMemAccess = nil
	cu.InFlightInstFetch = nil
	cu.InFlightVectorMemAccess = nil
}

func (cu *ComputeUnit) setWavesToReady() {
	for _, wfPool := range cu.WfPools {
		for _, wf := range wfPool.wfs {
			if wf.State != wavefront.WfCompleted {
				wf.State = wavefront.WfReady
				wf.IsFetching = false
			}
		}
	}
}
