package cu

import (
	"log"
	"reflect"

	"github.com/rs/xid"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/kernels"
	"github.com/sarchlab/mgpusim/v4/amd/protocol"
	"github.com/sarchlab/mgpusim/v4/amd/sampling"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
)

// A ComputeUnit in the timing package provides a detailed and accurate
// simulation of a GCN3 ComputeUnit
type ComputeUnit struct {
	*sim.TickingComponent

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

	running bool

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

	InstMem          sim.Port
	ScalarMem        sim.Port
	VectorMemModules mem.AddressToPortMapper

	ToACE sim.Port
	// toACESender sim.BufferedSender
	ToInstMem   sim.Port
	ToScalarMem sim.Port
	ToVectorMem sim.Port
	ToCP        sim.Port

	inCPRequestProcessingStage sim.Msg
	cpRequestHandlingComplete  bool

	isFlushing                   bool
	isPaused                     bool
	isSendingOutShadowBufferReqs bool
	isHandlingWfCompletionEvent  bool

	toSendToCP sim.Msg

	currentFlushReq   *protocol.CUPipelineFlushReq
	currentRestartReq *protocol.CUPipelineRestartReq
	//for sampling
	wftime map[string]sim.VTimeInSec
}

// ControlPort returns the port that can receive controlling messages from the
// Command Processor.
func (cu *ComputeUnit) ControlPort() sim.Port {
	return cu.ToCP
}

// DispatchingPort returns the port that the dispatcher can use to dispatch
// work-groups to the CU.
func (cu *ComputeUnit) DispatchingPort() sim.Port {
	return cu.ToACE
}

// WfPoolSizes returns an array of the numbers of wavefronts that each SIMD unit
// can execute.
func (cu *ComputeUnit) WfPoolSizes() []int {
	return []int{10, 10, 10, 10}
}

// VRegCounts returns an array of the numbers of vector regsiters in each SIMD
// unit.
func (cu *ComputeUnit) VRegCounts() []int {
	return []int{16384, 16384, 16384, 16384}
}

// SRegCount returns the number of scalar register in the Compute Unit.
func (cu *ComputeUnit) SRegCount() int {
	return 3200
}

// LDSBytes returns the number of bytes in the LDS of the CU.
func (cu *ComputeUnit) LDSBytes() int {
	return 64 * 1024
}

// Tick ticks
func (cu *ComputeUnit) Tick() bool {
	cu.Lock()
	defer cu.Unlock()

	madeProgress := false

	madeProgress = cu.runPipeline() || madeProgress
	// madeProgress = cu.sendToACE() || madeProgress
	madeProgress = cu.sendToCP() || madeProgress
	madeProgress = cu.processInput() || madeProgress
	madeProgress = cu.doFlush() || madeProgress

	return madeProgress
}

//nolint:gocyclo
func (cu *ComputeUnit) runPipeline() bool {
	madeProgress := false

	if !cu.isPaused {
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
	if cu.isFlushing {
		//If a flush request arrives before the shadow buffer requests have been sent out
		if cu.isSendingOutShadowBufferReqs {
			madeProgress = cu.reInsertShadowBufferReqsToOriginalBuffers() || madeProgress
		}
		madeProgress = cu.flushPipeline() || madeProgress
	}

	if cu.isSendingOutShadowBufferReqs {
		madeProgress = cu.checkShadowBuffers() || madeProgress
	}

	return madeProgress
}

func (cu *ComputeUnit) processInput() bool {
	madeProgress := false

	if !cu.isPaused || cu.isSendingOutShadowBufferReqs {
		madeProgress = cu.processInputFromACE() || madeProgress
		madeProgress = cu.processInputFromInstMem() || madeProgress
		madeProgress = cu.processInputFromScalarMem() || madeProgress
		madeProgress = cu.processInputFromVectorMem() || madeProgress
	}

	madeProgress = cu.processInputFromCP() || madeProgress

	return madeProgress
}

func (cu *ComputeUnit) processInputFromCP() bool {
	req := cu.ToCP.RetrieveIncoming()
	if req == nil {
		return false
	}

	cu.inCPRequestProcessingStage = req
	switch req := req.(type) {
	case *protocol.CUPipelineRestartReq:
		cu.handlePipelineResume(req)
	case *protocol.CUPipelineFlushReq:
		cu.handlePipelineFlushReq(req)
	default:
		panic("unknown msg type")
	}

	return true
}

func (cu *ComputeUnit) handlePipelineFlushReq(
	req *protocol.CUPipelineFlushReq,
) error {
	cu.isFlushing = true
	cu.currentFlushReq = req

	return nil
}

func (cu *ComputeUnit) handlePipelineResume(
	req *protocol.CUPipelineRestartReq,
) error {
	cu.isSendingOutShadowBufferReqs = true
	cu.currentRestartReq = req

	rsp := protocol.CUPipelineRestartRspBuilder{}.
		WithSrc(cu.ToCP.AsRemote()).
		WithDst(cu.currentRestartReq.Src).
		Build()
	err := cu.ToCP.Send(rsp)

	if err != nil {
		cu.currentRestartReq = nil
		log.Panicf("Unable to send restart rsp to CP")
	}
	return nil
}

func (cu *ComputeUnit) sendToCP() bool {
	if cu.toSendToCP == nil {
		return false
	}

	sendErr := cu.ToCP.Send(cu.toSendToCP)
	if sendErr == nil {
		cu.toSendToCP = nil
		return true
	}

	return false
}

func (cu *ComputeUnit) sendToACE(msg sim.Msg) bool {
	err := cu.ToACE.Send(msg)
	if err != nil {
		log.Panicf("Unable to send to ACE")
	}

	return true
}

func (cu *ComputeUnit) flushPipeline() bool {
	if cu.currentFlushReq == nil {
		return false
	}

	if cu.isHandlingWfCompletionEvent == true {
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
	cu.isPaused = true

	respondToCP := protocol.CUPipelineFlushRspBuilder{}.
		WithSrc(cu.ToCP.AsRemote()).
		WithDst(cu.currentFlushReq.Src).
		Build()
	cu.toSendToCP = respondToCP
	cu.currentFlushReq = nil
	cu.isFlushing = false

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
	req := cu.ToACE.RetrieveIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *protocol.MapWGReq:
		return cu.handleMapWGReq(req)
	default:
		panic("unknown req type")
	}
}

// Handle the wavefront completion events
func (cu *ComputeUnit) Handle(evt sim.Event) error {
	ctx := sim.HookCtx{
		Domain: cu,
		Pos:    sim.HookPosBeforeEvent,
		Item:   evt,
	}
	cu.InvokeHook(ctx)

	cu.Lock()

	defer cu.Unlock()

	switch evt := evt.(type) {
	case *wavefront.WfCompletionEvent:
		cu.handleWfCompletionEvent(evt)
	default:
		log.Panicf("Unable to process evevt of type %s",
			reflect.TypeOf(evt))
	}

	ctx.Pos = sim.HookPosAfterEvent
	cu.InvokeHook(ctx)

	return nil
}
func (cu *ComputeUnit) handleWfCompletionEvent(
	evt *wavefront.WfCompletionEvent,
) error {
	wf := evt.Wf
	wf.State = wavefront.WfCompleted
	sTmp := cu.Scheduler
	s := sTmp.(*SchedulerImpl)
	if s.areAllOtherWfsInWGCompleted(wf.WG, wf) {
		now := evt.Time()

		done := s.sendWGCompletionMessage(wf.WG)
		if !done {
			newEvent := wavefront.NewWfCompletionEvent(cu.Freq.NextTick(now), cu, wf)
			cu.Engine.Schedule(newEvent)
			return nil
		}

		s.resetRegisterValue(wf)
		cu.clearWGResource(wf.WG)
		tracing.EndTask(wf.UID, cu)
		tracing.TraceReqComplete(wf.WG.MapReq, cu)

		return nil
	}
	return nil
}
func (cu *ComputeUnit) handleMapWGReq(
	req *protocol.MapWGReq,
) bool {
	now := cu.CurrentTime()

	wg := cu.wrapWG(req.WorkGroup, req)

	tracing.TraceReqReceive(req, cu)

	//sampling
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
					predictedTime, cu, wf)
				cu.Engine.Schedule(newEvent)
				tracing.StartTask(wf.UID,
					tracing.MsgIDAtReceiver(req, cu),
					cu,
					"wavefront",
					"wavefront",
					nil,
				)
			}
		}
	}

	if !skipSimulate {
		for i, wf := range wg.Wfs {
			location := req.Wavefronts[i]
			cu.WfPools[location.SIMDID].AddWf(wf)
			cu.WfDispatcher.DispatchWf(wf, req.Wavefronts[i])
			wf.State = wavefront.WfReady

			tracing.StartTaskWithSpecificLocation(wf.UID,
				tracing.MsgIDAtReceiver(req, cu),
				cu,
				"wavefront",
				"wavefront",
				cu.Name()+".WFPool",
				nil,
			)
		}
	}

	cu.running = true
	cu.TickLater()

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
	req *protocol.MapWGReq,
) *wavefront.WorkGroup {
	wg := wavefront.NewWorkGroup(raw, req)

	lds := make([]byte, req.WorkGroup.Packet.GroupSegmentSize)
	wg.LDS = lds

	for _, rawWf := range req.WorkGroup.Wavefronts {
		wf := wavefront.NewWavefront(rawWf)
		wg.Wfs = append(wg.Wfs, wf)
		wf.WG = wg
		wf.SetPID(req.PID)
	}

	return wg
}

func (cu *ComputeUnit) processInputFromInstMem() bool {
	rsp := cu.ToInstMem.RetrieveIncoming()
	if rsp == nil {
		return false
	}

	switch rsp := rsp.(type) {
	case *mem.DataReadyRsp:
		cu.handleFetchReturn(rsp)
	default:
		log.Panicf("cannot handle request of type %s from ToInstMem port",
			reflect.TypeOf(rsp))
	}
	return true
}

func (cu *ComputeUnit) handleFetchReturn(
	rsp *mem.DataReadyRsp,
) bool {
	if len(cu.InFlightInstFetch) == 0 {
		return false
	}

	info := cu.InFlightInstFetch[0]
	if info.Req.ID != rsp.RespondTo {
		return false
	}

	wf := info.Wavefront
	addr := info.Address
	cu.InFlightInstFetch = cu.InFlightInstFetch[1:]

	if addr == wf.InstBufferStartPC+uint64(len(wf.InstBuffer)) {
		wf.InstBuffer = append(wf.InstBuffer, rsp.Data...)
	}

	wf.IsFetching = false
	wf.LastFetchTime = cu.TickingComponent.TickScheduler.CurrentTime()

	tracing.TraceReqFinalize(info.Req, cu)
	tracing.EndTask(info.Req.ID+"_fetch", cu)
	return true
}

func (cu *ComputeUnit) processInputFromScalarMem() bool {
	rsp := cu.ToScalarMem.RetrieveIncoming()
	if rsp == nil {
		return false
	}

	switch rsp := rsp.(type) {
	case *mem.DataReadyRsp:
		cu.handleScalarDataLoadReturn(rsp)
	default:
		log.Panicf("cannot handle request of type %s from ToInstMem port",
			reflect.TypeOf(rsp))
	}
	return true
}

func (cu *ComputeUnit) handleScalarDataLoadReturn(
	rsp *mem.DataReadyRsp,
) {
	if len(cu.InFlightScalarMemAccess) == 0 {
		return
	}

	info := cu.InFlightScalarMemAccess[0]
	req := info.Req
	if req.ID != rsp.RespondTo {
		return
	}

	wf := info.Wavefront
	access := RegisterAccess{
		WaveOffset: wf.SRegOffset,
		Reg:        info.DstSGPR,
		RegCount:   len(rsp.Data) / 4,
		Data:       rsp.Data,
	}
	cu.SRegFile.Write(access)

	cu.InFlightScalarMemAccess = cu.InFlightScalarMemAccess[1:]

	tracing.TraceReqFinalize(req, cu)

	if cu.isLastRead(req) {
		wf.OutstandingScalarMemAccess--
		cu.logInstTask(wf, info.Inst, true)
	}
}

func (cu *ComputeUnit) isLastRead(req *mem.ReadReq) bool {
	return !req.CanWaitForCoalesce
}

func (cu *ComputeUnit) processInputFromVectorMem() bool {
	rsp := cu.ToVectorMem.RetrieveIncoming()
	if rsp == nil {
		return false
	}

	switch rsp := rsp.(type) {
	case *mem.DataReadyRsp:
		cu.handleVectorDataLoadReturn(rsp)
	case *mem.WriteDoneRsp:
		cu.handleVectorDataStoreRsp(rsp)
	default:
		log.Panicf("cannot handle request of type %s from ToInstMem port",
			reflect.TypeOf(rsp))
	}

	return true
}

//nolint:gocyclo
func (cu *ComputeUnit) handleVectorDataLoadReturn(
	rsp *mem.DataReadyRsp,
) {
	if len(cu.InFlightVectorMemAccess) == 0 {
		return
	}

	info := cu.InFlightVectorMemAccess[0]

	if info.Read == nil {
		return
	}

	if info.Read.ID != rsp.RespondTo {
		return
	}

	cu.InFlightVectorMemAccess = cu.InFlightVectorMemAccess[1:]
	tracing.TraceReqFinalize(info.Read, cu)

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
			access.Data = rsp.Data[offset : offset+uint64(4*laneInfo.regCount)]
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
	rsp *mem.WriteDoneRsp,
) {
	if len(cu.InFlightVectorMemAccess) == 0 {
		return
	}

	info := cu.InFlightVectorMemAccess[0]

	if info.Write == nil {
		return
	}

	if info.Write.ID != rsp.RespondTo {
		return
	}

	cu.InFlightVectorMemAccess = cu.InFlightVectorMemAccess[1:]
	tracing.TraceReqFinalize(info.Write, cu)

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
	wf.PC += uint64(wf.Inst().ByteSize)
	cu.removeStaleInstBuffer(wf)
}

func (cu *ComputeUnit) removeStaleInstBuffer(wf *wavefront.Wavefront) {
	if len(wf.InstBuffer) != 0 {
		for wf.PC >= wf.InstBufferStartPC+64 {
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
		tracing.EndTask(inst.ID, cu)
		return
	}

	tracing.StartTaskWithSpecificLocation(
		inst.ID,
		wf.UID,
		cu,
		"inst",
		cu.execUnitToString(inst.ExeUnit),
		cu.Name()+"."+cu.execUnitToString(inst.ExeUnit),
		// inst.InstName,
		map[string]interface{}{
			"inst": inst,
			"wf":   wf,
		},
	)
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
	cu.isSendingOutShadowBufferReqs = false
	for i := 0; i < len(cu.shadowInFlightVectorMemAccess); i++ {
		cu.InFlightVectorMemAccess = append(cu.InFlightVectorMemAccess, cu.shadowInFlightVectorMemAccess[i])
	}

	for i := 0; i < len(cu.shadowInFlightScalarMemAccess); i++ {
		cu.InFlightScalarMemAccess = append(cu.InFlightScalarMemAccess, cu.shadowInFlightScalarMemAccess[i])
	}

	for i := 0; i < len(cu.shadowInFlightInstFetch); i++ {
		cu.InFlightInstFetch = append(cu.InFlightInstFetch, cu.shadowInFlightInstFetch[i])
	}

	return true
}

func (cu *ComputeUnit) checkShadowBuffers() bool {
	numReqsPendingToSend :=
		len(cu.shadowInFlightScalarMemAccess) +
			len(cu.shadowInFlightVectorMemAccess) +
			len(cu.shadowInFlightInstFetch)

	if numReqsPendingToSend == 0 {
		cu.isSendingOutShadowBufferReqs = false
		cu.Scheduler.Resume()
		cu.isPaused = false
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

		req := info.Req
		req.ID = xid.New().String()
		err := cu.ToScalarMem.Send(req)
		if err == nil {
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
			req := info.Read
			req.ID = sim.GetIDGenerator().Generate()
			err := cu.ToVectorMem.Send(req)
			if err == nil {
				cu.InFlightVectorMemAccess = append(
					cu.InFlightVectorMemAccess, info)
				cu.shadowInFlightVectorMemAccess = cu.shadowInFlightVectorMemAccess[1:]
				return true
			}
		} else if info.Write != nil {
			req := info.Write
			req.ID = sim.GetIDGenerator().Generate()
			err := cu.ToVectorMem.Send(req)
			if err == nil {
				cu.InFlightVectorMemAccess = append(cu.InFlightVectorMemAccess, info)
				cu.shadowInFlightVectorMemAccess = cu.shadowInFlightVectorMemAccess[1:]
				return true
			}
		}
	}
	return false
}

func (cu *ComputeUnit) sendInstFetchShadowBufferAccesses() bool {
	if len(cu.shadowInFlightInstFetch) > 0 {
		info := cu.shadowInFlightInstFetch[0]
		req := info.Req
		req.ID = xid.New().String()
		err := cu.ToInstMem.Send(req)
		if err == nil {
			cu.InFlightInstFetch = append(cu.InFlightInstFetch, info)
			cu.shadowInFlightInstFetch = cu.shadowInFlightInstFetch[1:]
			return true
		}
	}
	return false
}
func (cu *ComputeUnit) populateShadowBuffers() {
	for i := 0; i < len(cu.InFlightInstFetch); i++ {
		cu.shadowInFlightInstFetch = append(cu.shadowInFlightInstFetch, cu.InFlightInstFetch[i])
	}

	for i := 0; i < len(cu.InFlightScalarMemAccess); i++ {
		cu.shadowInFlightScalarMemAccess = append(cu.shadowInFlightScalarMemAccess, cu.InFlightScalarMemAccess[i])
	}

	for i := 0; i < len(cu.InFlightVectorMemAccess); i++ {
		cu.shadowInFlightVectorMemAccess = append(cu.shadowInFlightVectorMemAccess, cu.InFlightVectorMemAccess[i])
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

// NewComputeUnit returns a newly constructed compute unit
func NewComputeUnit(
	name string,
	engine sim.Engine,
) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.TickingComponent = sim.NewTickingComponent(
		name, engine, 1*sim.GHz, cu)

	cu.ToACE = sim.NewPort(cu, 4, 4, name+".ToACE")
	cu.ToInstMem = sim.NewPort(cu, 4, 4, name+".ToInstMem")
	cu.ToScalarMem = sim.NewPort(cu, 4, 4, name+".ToScalarMem")
	cu.ToVectorMem = sim.NewPort(cu, 4, 4, name+".ToVectorMem")
	cu.ToCP = sim.NewPort(cu, 4, 4, name+".ToCP")

	cu.AddPort("Top", cu.ToACE)
	cu.AddPort("Ctrl", cu.ToCP)
	cu.AddPort("InstMem", cu.ToInstMem)
	cu.AddPort("ScalarMem", cu.ToScalarMem)
	cu.AddPort("VectorMem", cu.ToVectorMem)

	cu.wftime = make(map[string]sim.VTimeInSec)

	return cu
}
