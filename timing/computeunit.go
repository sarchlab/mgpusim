package timing

import (
	"log"
	"reflect"

	"gitlab.com/akita/vis/trace"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/emu"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
	"gitlab.com/akita/gcn3/timing/wavefront"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/util/tracing"
)

// A ComputeUnit in the timing package provides a detailed and accurate
// simulation of a GCN3 ComputeUnit
type ComputeUnit struct {
	*akita.TickingComponent

	WGMapper     WGMapper
	WfDispatcher WfDispatcher
	Decoder      emu.Decoder

	WfPools              []*WavefrontPool
	WfToDispatch         map[*kernels.Wavefront]*WfDispatchInfo
	wgToManagedWgMapping map[*kernels.WorkGroup]*wavefront.WorkGroup

	InFlightInstFetch       []*InstFetchReqInfo
	InFlightScalarMemAccess []*ScalarMemAccessInfo
	InFlightVectorMemAccess []VectorMemAccessInfo

	running bool

	Scheduler        Scheduler
	BranchUnit       CUComponent
	VectorMemDecoder CUComponent
	VectorMemUnit    CUComponent
	ScalarDecoder    CUComponent
	VectorDecoder    CUComponent
	LDSDecoder       CUComponent
	ScalarUnit       CUComponent
	SIMDUnit         []CUComponent
	LDSUnit          CUComponent
	SRegFile         RegisterFile
	VRegFile         []RegisterFile

	InstMem          akita.Port
	ScalarMem        akita.Port
	VectorMemModules cache.LowModuleFinder

	ToACE       akita.Port
	ToInstMem   akita.Port
	ToScalarMem akita.Port
	ToVectorMem akita.Port

	ToCP akita.Port
	CP   akita.Port

	inCPRequestProcessingStage akita.Req
	cpRequestHandlingComplete  bool

	isDraining bool
	isFlushing bool
	isPaused   bool

	flushLatency   uint64
	flushCycleLeft uint64

	toSendToCP akita.Req

	currentFlushReq *gcn3.CUPipelineFlushReq
}

// Handle processes that events that are scheduled on the ComputeUnit
func (cu *ComputeUnit) Handle(evt akita.Event) error {
	ctx := akita.HookCtx{
		Domain: cu,
		Now:    evt.Time(),
		Pos:    akita.HookPosBeforeEvent,
		Item:   evt,
	}
	cu.InvokeHook(&ctx)

	cu.Lock()

	switch evt := evt.(type) {
	case akita.TickEvent:
		cu.handleTickEvent(evt)
	case *WfDispatchEvent:
		cu.handleWfDispatchEvent(evt)
	case *WfCompletionEvent:
		cu.handleWfCompletionEvent(evt)
	default:
		cu.Unlock()
		log.Panicf("Unable to process evevt of type %s",
			reflect.TypeOf(evt))
	}

	cu.Unlock()

	ctx.Pos = akita.HookPosAfterEvent
	cu.InvokeHook(&ctx)

	return nil
}

func (cu *ComputeUnit) handleTickEvent(evt akita.TickEvent) {
	now := evt.Time()
	cu.NeedTick = false

	cu.runPipeline(now)
	cu.processInput(now)
	if cu.isDraining {
		cu.drainPipeline(now)
	}
	if cu.isFlushing {
		cu.flushPipeline(now)
	}
	cu.sendToCP(now)

	if cu.NeedTick {
		cu.TickLater(now)
	}
}

func (cu *ComputeUnit) runPipeline(now akita.VTimeInSec) {
	madeProgress := false

	madeProgress = cu.BranchUnit.Run(now) || madeProgress

	madeProgress = cu.ScalarUnit.Run(now) || madeProgress
	madeProgress = cu.ScalarDecoder.Run(now) || madeProgress

	for _, simdUnit := range cu.SIMDUnit {
		madeProgress = simdUnit.Run(now) || madeProgress
	}
	madeProgress = cu.VectorDecoder.Run(now) || madeProgress

	madeProgress = cu.LDSUnit.Run(now) || madeProgress
	madeProgress = cu.LDSDecoder.Run(now) || madeProgress

	madeProgress = cu.VectorMemUnit.Run(now) || madeProgress
	madeProgress = cu.VectorMemDecoder.Run(now) || madeProgress

	madeProgress = cu.Scheduler.Run(now) || madeProgress

	if madeProgress {
		cu.NeedTick = true
	}
}

func (cu *ComputeUnit) processInput(now akita.VTimeInSec) {

	if !cu.isPaused {
		cu.processInputFromACE(now)
		cu.processInputFromInstMem(now)
		cu.processInputFromScalarMem(now)
		cu.processInputFromVectorMem(now)
	}
	//When pausing we still allow requests from CP so that we can receive the resume command correctly

	cu.processInputFromCP(now)
}

func (cu *ComputeUnit) processInputFromCP(now akita.VTimeInSec) {

	req := cu.ToCP.Retrieve(now)

	if req == nil {
		return
	}

	cu.NeedTick = true

	cu.inCPRequestProcessingStage = req

	switch req := req.(type) {
	case *gcn3.CUPipelineDrainReq:
		cu.handlePipelineDrainReq(now, req)
	case *gcn3.CUPipelineRestart:
		cu.handlePipelineResume(now, req)
	case *gcn3.CUPipelineFlushReq:
		cu.handlePipelineFlushReq(now, req)
	}
}

func (cu *ComputeUnit) handlePipelineDrainReq(
	now akita.VTimeInSec,
	req *gcn3.CUPipelineDrainReq,
) error {
	//1. Issue drain command to scheduler
	//2. Check all units one by one until all idle
	//3. If all complete issue CU Pipeline Drain Completion respond
	cu.isDraining = true

	cu.Scheduler.Pause()

	return nil
}

func (cu *ComputeUnit) handlePipelineFlushReq(
	now akita.VTimeInSec,
	req *gcn3.CUPipelineFlushReq,
) error {

	cu.isFlushing = true
	cu.currentFlushReq = req
	cu.flushCycleLeft = cu.flushLatency

	return nil
}

func (cu *ComputeUnit) handlePipelineResume(
	now akita.VTimeInSec,
	req *gcn3.CUPipelineRestart,
) error {
	//1. Issue drain command to scheduler
	//2. Check all units one by one until all idle
	//3. If all complete issue CU Pipeline Drain Completion respond
	cu.Scheduler.Resume()
	cu.isPaused = false
	return nil

}

func (cu *ComputeUnit) sendToCP(now akita.VTimeInSec) {
	if cu.toSendToCP == nil {
		return
	}
	cu.toSendToCP.SetSendTime(now)
	sendErr := cu.ToCP.Send(cu.toSendToCP)
	if sendErr == nil {
		cu.toSendToCP = nil
		cu.NeedTick = true
	}

}

func (cu *ComputeUnit) drainPipeline(now akita.VTimeInSec) {
	drainCompleted := true

	drainCompleted = drainCompleted && cu.BranchUnit.IsIdle()

	drainCompleted = drainCompleted && cu.ScalarUnit.IsIdle()
	drainCompleted = drainCompleted && cu.ScalarDecoder.IsIdle()

	for _, simdUnit := range cu.SIMDUnit {
		drainCompleted = drainCompleted && simdUnit.IsIdle()
	}

	drainCompleted = drainCompleted && cu.VectorDecoder.IsIdle()

	drainCompleted = drainCompleted && cu.LDSUnit.IsIdle()
	drainCompleted = drainCompleted && cu.LDSDecoder.IsIdle()

	drainCompleted = drainCompleted && cu.VectorMemUnit.IsIdle()
	drainCompleted = drainCompleted && cu.VectorMemDecoder.IsIdle()

	drainCompleted = drainCompleted && (len(cu.InFlightInstFetch) == 0) && (len(cu.InFlightScalarMemAccess) == 0) && (len(cu.InFlightVectorMemAccess) == 0)

	if drainCompleted == true {
		respondToCP := gcn3.NewCUPipelineDrainRsp(now, cu.ToCP, cu.CP)
		cu.toSendToCP = respondToCP
		cu.isDraining = false
	}

}

func (cu *ComputeUnit) flushPipeline(now akita.VTimeInSec) {

	if cu.currentFlushReq == nil {
		return
	}

	if cu.flushCycleLeft <= 0 {

		cu.flushCUBuffers()
		cu.Scheduler.Flush()

		cu.flushInternalComponents()

		cu.currentFlushReq = nil

		respondToCP := gcn3.NewCUPipelineFlushRsp(now, cu.ToCP, cu.CP)
		cu.toSendToCP = respondToCP

		cu.isFlushing = false
		cu.isPaused = true

	}

	cu.flushCycleLeft--
	cu.NeedTick = true
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

func (cu *ComputeUnit) processInputFromACE(now akita.VTimeInSec) {
	req := cu.ToACE.Retrieve(now)
	if req == nil {
		return
	}

	switch req := req.(type) {
	case *gcn3.MapWGReq:
		cu.handleMapWGReq(now, req)
	}
}

func (cu *ComputeUnit) handleMapWGReq(
	now akita.VTimeInSec,
	req *gcn3.MapWGReq,
) error {
	//log.Printf("%s map wg at %.12f\n", cu.Name(), req.Time())

	ok := false

	if len(cu.WfToDispatch) == 0 {
		ok = cu.WGMapper.MapWG(req)
	}

	wfs := make([]*wavefront.Wavefront, 0)
	if ok {
		cu.wrapWG(req.WG, req)
		for _, wf := range req.WG.Wavefronts {
			managedWf := cu.wrapWf(wf)
			managedWf.SetPID(req.PID)
			wfs = append(wfs, managedWf)
		}

		for i, wf := range wfs {
			evt := NewWfDispatchEvent(cu.Freq.NCyclesLater(i, now), cu, wf)
			evt.MapWGReq = req
			cu.Engine.Schedule(evt)
		}

		lastEventCycle := 4
		if len(wfs) > 4 {
			lastEventCycle = len(wfs)
		}
		evt := NewWfDispatchEvent(cu.Freq.NCyclesLater(lastEventCycle, now), cu, nil)
		evt.MapWGReq = req
		evt.IsLastInWG = true
		cu.Engine.Schedule(evt)

		task := trace.Task{
			ID: req.GetID(),
		}
		ctx := akita.HookCtx{
			Domain: cu,
			Now:    now,
			Pos:    trace.HookPosTaskStart,
			Item:   task,
		}
		cu.InvokeHook(&ctx)

		return nil
	}

	req.Ok = false
	req.SwapSrcAndDst()
	req.SetSendTime(now)
	err := cu.ToACE.Send(req)
	if err != nil {
		log.Panic(err)
	}

	return nil
}

func (cu *ComputeUnit) handleWfDispatchEvent(
	evt *WfDispatchEvent,
) error {
	now := evt.Time()
	wf := evt.ManagedWf
	if wf != nil {
		info := cu.WfToDispatch[wf.Wavefront]
		cu.WfPools[info.SIMDID].AddWf(wf)
		cu.WfDispatcher.DispatchWf(now, wf)
		delete(cu.WfToDispatch, wf.Wavefront)

		wf.State = wavefront.WfReady

		task := trace.Task{
			ID:           wf.UID,
			ParentID:     evt.MapWGReq.GetID(),
			Type:         "Wavefront",
			What:         "Wavefront",
			Where:        cu.Name(),
			InitiateTime: float64(now),
		}
		ctx := akita.HookCtx{
			Domain: cu,
			Now:    now,
			Pos:    trace.HookPosTaskInitiate,
			Item:   task,
		}
		cu.InvokeHook(&ctx)
	}

	// Respond ACK
	if evt.IsLastInWG {
		req := evt.MapWGReq
		req.Ok = true
		req.SwapSrcAndDst()
		req.SetSendTime(evt.Time())
		err := cu.ToACE.Send(req)
		if err != nil {
			log.Panic(err)
		}
	}

	cu.running = true
	cu.TickLater(now)

	return nil
}

func (cu *ComputeUnit) handleWfCompletionEvent(evt *WfCompletionEvent) error {
	now := evt.Time()
	wf := evt.Wf
	wg := wf.WG
	wf.State = wavefront.WfCompleted

	task := trace.Task{
		ID: wf.UID,
	}
	ctx := akita.HookCtx{
		Domain: cu,
		Now:    now,
		Pos:    trace.HookPosTaskClear,
		Item:   task,
	}
	cu.InvokeHook(&ctx)

	if cu.isAllWfInWGCompleted(wg) {
		ok := cu.sendWGCompletionMessage(evt, wg)
		if ok {
			cu.clearWGResource(wg)

			task := trace.Task{
				ID: wg.MapReq.GetID(),
			}
			ctx := akita.HookCtx{
				Domain: cu,
				Now:    now,
				Pos:    trace.HookPosTaskComplete,
				Item:   task,
			}
			cu.InvokeHook(&ctx)
		}

		if !cu.hasMoreWfsToRun() {
			cu.running = false
		}
	}
	cu.TickLater(evt.Time())

	return nil
}

func (cu *ComputeUnit) clearWGResource(wg *wavefront.WorkGroup) {
	cu.WGMapper.UnmapWG(wg)
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

func (cu *ComputeUnit) sendWGCompletionMessage(
	evt *WfCompletionEvent,
	wg *wavefront.WorkGroup,
) bool {
	mapReq := wg.MapReq
	dispatcher := mapReq.Dst() // This is dst since the mapReq has been sent back already
	now := evt.Time()
	mesg := gcn3.NewWGFinishMesg(cu.ToACE, dispatcher, now, wg.WorkGroup)

	err := cu.ToACE.Send(mesg)
	if err != nil {
		newEvent := NewWfCompletionEvent(
			cu.Freq.NextTick(now), cu, evt.Wf)
		cu.Engine.Schedule(newEvent)
		return false
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
	req *gcn3.MapWGReq,
) *wavefront.WorkGroup {
	wg := wavefront.NewWorkGroup(raw, req)

	lds := make([]byte, wg.CodeObject.WGGroupSegmentByteSize)
	wg.LDS = lds

	cu.wgToManagedWgMapping[raw] = wg
	return wg
}

func (cu *ComputeUnit) wrapWf(raw *kernels.Wavefront) *wavefront.Wavefront {
	wf := wavefront.NewWavefront(raw)
	wg := cu.wgToManagedWgMapping[raw.WG]
	wg.Wfs = append(wg.Wfs, wf)
	wf.WG = wg
	return wf
}

func (cu *ComputeUnit) processInputFromInstMem(now akita.VTimeInSec) {
	rsp := cu.ToInstMem.Retrieve(now)
	if rsp == nil {
		return
	}
	cu.NeedTick = true

	switch rsp := rsp.(type) {
	case *mem.DataReadyRsp:
		cu.handleFetchReturn(now, rsp)
	default:
		log.Panicf("cannot handle request of type %s from ToInstMem port",
			reflect.TypeOf(rsp))
	}
}

func (cu *ComputeUnit) handleFetchReturn(now akita.VTimeInSec, rsp *mem.DataReadyRsp) {
	if len(cu.InFlightInstFetch) == 0 {
		log.Panic("CU is fetching no instruction")
	}

	info := cu.InFlightInstFetch[0]
	if info.Req.ID != rsp.RespondTo {
		log.Panic("response does not match request")
	}

	wf := info.Wavefront
	addr := info.Address
	cu.InFlightInstFetch = cu.InFlightInstFetch[1:]

	if addr == wf.InstBufferStartPC+uint64(len(wf.InstBuffer)) {
		wf.InstBuffer = append(wf.InstBuffer, rsp.Data...)
	}

	wf.IsFetching = false
	wf.LastFetchTime = now
}

func (cu *ComputeUnit) processInputFromScalarMem(now akita.VTimeInSec) {
	rsp := cu.ToScalarMem.Retrieve(now)
	if rsp == nil {
		return
	}
	cu.NeedTick = true

	switch rsp := rsp.(type) {
	case *mem.DataReadyRsp:
		cu.handleScalarDataLoadReturn(now, rsp)
	default:
		log.Panicf("cannot handle request of type %s from ToInstMem port",
			reflect.TypeOf(rsp))
	}
}

func (cu *ComputeUnit) handleScalarDataLoadReturn(
	now akita.VTimeInSec,
	rsp *mem.DataReadyRsp,
) {
	if len(cu.InFlightScalarMemAccess) == 0 {
		log.Panic("CU is not loading scalar data")
	}

	info := cu.InFlightScalarMemAccess[0]
	if info.Req.ID != rsp.RespondTo {
		log.Panic("response does not match request")
	}

	wf := info.Wavefront
	access := RegisterAccess{}
	access.WaveOffset = wf.SRegOffset
	access.Reg = info.DstSGPR
	access.RegCount = int(len(rsp.Data) / 4)
	access.Data = rsp.Data
	cu.SRegFile.Write(access)

	wf.OutstandingScalarMemAccess--
	cu.InFlightScalarMemAccess = cu.InFlightScalarMemAccess[1:]

	cu.logInstStageTask(now, info.Inst, "mem", true)
	cu.logInstTask(now, wf, info.Inst, true)
}

func (cu *ComputeUnit) processInputFromVectorMem(now akita.VTimeInSec) {
	rsp := cu.ToVectorMem.Retrieve(now)
	if rsp == nil {
		return
	}
	cu.NeedTick = true

	switch rsp := rsp.(type) {
	case *mem.DataReadyRsp:
		cu.handleVectorDataLoadReturn(now, rsp)
	case *mem.DoneRsp:
		cu.handleVectorDataStoreRsp(now, rsp)
	default:
		log.Panicf("cannot handle request of type %s from ToInstMem port",
			reflect.TypeOf(rsp))
	}
}

func (cu *ComputeUnit) handleVectorDataLoadReturn(
	now akita.VTimeInSec,
	rsp *mem.DataReadyRsp,
) {
	if len(cu.InFlightVectorMemAccess) == 0 {
		log.Panic("CU is not accessing vector memory")
	}

	info := cu.InFlightVectorMemAccess[0]
	if info.Read.ID != rsp.RespondTo {
		log.Panic("CU cannot receive out of order memory return")
	}
	cu.InFlightVectorMemAccess = cu.InFlightVectorMemAccess[1:]
	tracing.TraceReqFinalize(info.Read, now, cu)

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

	if info.Read.IsLastInWave {
		wf.OutstandingVectorMemAccess--
		if info.Inst.FormatType == insts.FLAT {
			wf.OutstandingScalarMemAccess--
		}

		cu.logInstStageTask(now, info.Inst, "mem", true)
		cu.logInstTask(now, wf, info.Inst, true)
	}
}

func (cu *ComputeUnit) handleVectorDataStoreRsp(
	now akita.VTimeInSec,
	rsp *mem.DoneRsp,
) {
	info := cu.InFlightVectorMemAccess[0]
	if info.Write.ID != rsp.RespondTo {
		log.Panic("CU cannot receive out of order memory return")
	}
	cu.InFlightVectorMemAccess = cu.InFlightVectorMemAccess[1:]
	tracing.TraceReqFinalize(info.Write, now, cu)

	wf := info.Wavefront
	if info.Write.IsLastInWave {
		wf.OutstandingVectorMemAccess--
		if info.Inst.FormatType == insts.FLAT {
			wf.OutstandingScalarMemAccess--
		}
		cu.logInstStageTask(now, info.Inst, "mem", true)
		cu.logInstTask(now, wf, info.Inst, true)
	}
}

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
	now akita.VTimeInSec,
	wf *wavefront.Wavefront,
	inst *wavefront.Inst,
	completed bool,
) {
	if completed {
		tracing.EndTask(
			inst.ID,
			now,
			cu,
		)
		return
	}

	tracing.StartTask(
		inst.ID,
		wf.UID,
		now,
		cu,
		"inst",
		"inst",
		map[string]interface{}{
			"inst": inst,
			"wf":   wf,
		},
	)
}

func (cu *ComputeUnit) logInstStageTask(
	now akita.VTimeInSec,
	inst *wavefront.Inst,
	stage string,
	completed bool,
) {
	if len(cu.Hooks) > 0 {
		task := trace.Task{
			ID:           inst.ID + "_" + stage,
			ParentID:     inst.ID,
			Type:         "Inst Stage",
			What:         stage,
			Where:        cu.Name(),
			InitiateTime: float64(now),
		}

		ctx := akita.HookCtx{
			Domain: cu,
			Now:    now,
			Item:   task,
		}
		if completed {
			ctx.Pos = trace.HookPosTaskClear
		} else {
			task.InitiateTime = float64(now)
			ctx.Pos = trace.HookPosTaskInitiate
		}

		cu.InvokeHook(&ctx)
	}
}

// NewComputeUnit returns a newly constructed compute unit
func NewComputeUnit(
	name string,
	engine akita.Engine,
) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.TickingComponent = akita.NewTickingComponent(
		name, engine, 1*akita.GHz, cu)

	cu.WfToDispatch = make(map[*kernels.Wavefront]*WfDispatchInfo)
	cu.wgToManagedWgMapping = make(map[*kernels.WorkGroup]*wavefront.WorkGroup)

	cu.ToACE = akita.NewLimitNumReqPort(cu, 4)
	cu.ToInstMem = akita.NewLimitNumReqPort(cu, 4)
	cu.ToScalarMem = akita.NewLimitNumReqPort(cu, 4)
	cu.ToVectorMem = akita.NewLimitNumReqPort(cu, 4)

	cu.ToCP = akita.NewLimitNumReqPort(cu, 4)
	cu.CP = akita.NewLimitNumReqPort(cu, 4)

	cu.flushLatency = 1000

	return cu
}
