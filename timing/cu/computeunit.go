package cu

import (
	"log"
	"reflect"

	"github.com/rs/xid"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/emu"
	"github.com/sarchlab/mgpusim/v3/insts"
	"github.com/sarchlab/mgpusim/v3/kernels"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
	"github.com/sarchlab/mgpusim/v3/protocol"
	"github.com/sarchlab/mgpusim/v3/timing/wavefront"
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
	VectorMemModules mem.LowModuleFinder

	ToACE       sim.Port
	toACESender sim.BufferedSender
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
func (cu *ComputeUnit) Tick(now sim.VTimeInSec) bool {
	cu.Lock()
	defer cu.Unlock()

	madeProgress := false

	madeProgress = cu.runPipeline(now) || madeProgress
	madeProgress = cu.sendToACE(now) || madeProgress
	madeProgress = cu.sendToCP(now) || madeProgress
	madeProgress = cu.processInput(now) || madeProgress
	madeProgress = cu.doFlush(now) || madeProgress

	return madeProgress
}

//nolint:gocyclo
func (cu *ComputeUnit) runPipeline(now sim.VTimeInSec) bool {
	madeProgress := false

	if !cu.isPaused {
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
	}

	return madeProgress
}

func (cu *ComputeUnit) doFlush(now sim.VTimeInSec) bool {
	madeProgress := false
	if cu.isFlushing {
		//If a flush request arrives before the shadow buffer requests have been sent out
		if cu.isSendingOutShadowBufferReqs {
			madeProgress = cu.reInsertShadowBufferReqsToOriginalBuffers() || madeProgress
		}
		madeProgress = cu.flushPipeline(now) || madeProgress
	}

	if cu.isSendingOutShadowBufferReqs {
		madeProgress = cu.checkShadowBuffers(now) || madeProgress
	}

	return madeProgress
}

func (cu *ComputeUnit) processInput(now sim.VTimeInSec) bool {
	madeProgress := false

	if !cu.isPaused || cu.isSendingOutShadowBufferReqs {
		madeProgress = cu.processInputFromACE(now) || madeProgress
		madeProgress = cu.processInputFromInstMem(now) || madeProgress
		madeProgress = cu.processInputFromScalarMem(now) || madeProgress
		madeProgress = cu.processInputFromVectorMem(now) || madeProgress
	}

	madeProgress = cu.processInputFromCP(now) || madeProgress

	return madeProgress
}

func (cu *ComputeUnit) processInputFromCP(now sim.VTimeInSec) bool {
	req := cu.ToCP.Retrieve(now)
	if req == nil {
		return false
	}

	cu.inCPRequestProcessingStage = req
	switch req := req.(type) {
	case *protocol.CUPipelineRestartReq:
		cu.handlePipelineResume(now, req)
	case *protocol.CUPipelineFlushReq:
		cu.handlePipelineFlushReq(now, req)
	default:
		panic("unknown msg type")
	}

	return true
}

func (cu *ComputeUnit) handlePipelineFlushReq(
	now sim.VTimeInSec,
	req *protocol.CUPipelineFlushReq,
) error {
	cu.isFlushing = true
	cu.currentFlushReq = req

	return nil
}

func (cu *ComputeUnit) handlePipelineResume(
	now sim.VTimeInSec,
	req *protocol.CUPipelineRestartReq,
) error {
	cu.isSendingOutShadowBufferReqs = true
	cu.currentRestartReq = req

	rsp := protocol.CUPipelineRestartRspBuilder{}.
		WithSrc(cu.ToCP).
		WithDst(cu.currentRestartReq.Src).
		WithSendTime(now).
		Build()
	err := cu.ToCP.Send(rsp)

	if err != nil {
		cu.currentRestartReq = nil
		log.Panicf("Unable to send restart rsp to CP")
	}
	return nil
}

func (cu *ComputeUnit) sendToCP(now sim.VTimeInSec) bool {
	if cu.toSendToCP == nil {
		return false
	}

	cu.toSendToCP.Meta().SendTime = now
	sendErr := cu.ToCP.Send(cu.toSendToCP)
	if sendErr == nil {
		cu.toSendToCP = nil
		return true
	}

	return false
}

func (cu *ComputeUnit) sendToACE(now sim.VTimeInSec) bool {
	return cu.toACESender.Tick(now)
}

func (cu *ComputeUnit) flushPipeline(now sim.VTimeInSec) bool {
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
		WithSendTime(now).
		WithSrc(cu.ToCP).
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

func (cu *ComputeUnit) processInputFromACE(now sim.VTimeInSec) bool {
	req := cu.ToACE.Retrieve(now)
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *protocol.MapWGReq:
		return cu.handleMapWGReq(now, req)
	default:
		panic("unknown req type")
	}
}

func (cu *ComputeUnit) handleMapWGReq(
	now sim.VTimeInSec,
	req *protocol.MapWGReq,
) bool {
	wg := cu.wrapWG(req.WorkGroup, req)

	tracing.TraceReqReceive(req, cu)

	for i, wf := range wg.Wfs {
		location := req.Wavefronts[i]
		cu.WfPools[location.SIMDID].AddWf(wf)
		cu.WfDispatcher.DispatchWf(now, wf, req.Wavefronts[i])
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

	cu.running = true
	cu.TickLater(now)

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

func (cu *ComputeUnit) processInputFromInstMem(now sim.VTimeInSec) bool {
	rsp := cu.ToInstMem.Retrieve(now)
	if rsp == nil {
		return false
	}

	switch rsp := rsp.(type) {
	case *mem.DataReadyRsp:
		cu.handleFetchReturn(now, rsp)
	default:
		log.Panicf("cannot handle request of type %s from ToInstMem port",
			reflect.TypeOf(rsp))
	}
	return true
}

func (cu *ComputeUnit) handleFetchReturn(
	now sim.VTimeInSec,
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
	wf.LastFetchTime = now

	tracing.TraceReqFinalize(info.Req, cu)
	tracing.EndTask(info.Req.ID+"_fetch", cu)
	return true
}

func (cu *ComputeUnit) processInputFromScalarMem(now sim.VTimeInSec) bool {
	rsp := cu.ToScalarMem.Retrieve(now)
	if rsp == nil {
		return false
	}

	switch rsp := rsp.(type) {
	case *mem.DataReadyRsp:
		cu.handleScalarDataLoadReturn(now, rsp)
	default:
		log.Panicf("cannot handle request of type %s from ToInstMem port",
			reflect.TypeOf(rsp))
	}
	return true
}

func (cu *ComputeUnit) handleScalarDataLoadReturn(
	now sim.VTimeInSec,
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
	cu.logInstTask(now, wf, info.Inst, true)

	if cu.isLastRead(req) {
		wf.OutstandingScalarMemAccess--
	}
}

func (cu *ComputeUnit) isLastRead(req *mem.ReadReq) bool {
	return !req.CanWaitForCoalesce
}

func (cu *ComputeUnit) processInputFromVectorMem(now sim.VTimeInSec) bool {
	rsp := cu.ToVectorMem.Retrieve(now)
	if rsp == nil {
		return false
	}

	switch rsp := rsp.(type) {
	case *mem.DataReadyRsp:
		cu.handleVectorDataLoadReturn(now, rsp)
	case *mem.WriteDoneRsp:
		cu.handleVectorDataStoreRsp(now, rsp)
	default:
		log.Panicf("cannot handle request of type %s from ToInstMem port",
			reflect.TypeOf(rsp))
	}

	return true
}

//nolint:gocyclo
func (cu *ComputeUnit) handleVectorDataLoadReturn(
	now sim.VTimeInSec,
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

		cu.logInstTask(now, wf, info.Inst, true)
	}
}

func (cu *ComputeUnit) handleVectorDataStoreRsp(
	now sim.VTimeInSec,
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
		cu.logInstTask(now, wf, info.Inst, true)
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
	now sim.VTimeInSec,
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

func (cu *ComputeUnit) checkShadowBuffers(now sim.VTimeInSec) bool {
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

	return cu.sendOutShadowBufferReqs(now)
}

func (cu *ComputeUnit) sendOutShadowBufferReqs(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = cu.sendScalarShadowBufferAccesses(now) || madeProgress
	madeProgress = cu.sendVectorShadowBufferAccesses(now) || madeProgress
	madeProgress = cu.sendInstFetchShadowBufferAccesses(now) || madeProgress

	return madeProgress
}

func (cu *ComputeUnit) sendScalarShadowBufferAccesses(
	now sim.VTimeInSec,
) bool {
	if len(cu.shadowInFlightScalarMemAccess) > 0 {
		info := cu.shadowInFlightScalarMemAccess[0]

		req := info.Req
		req.ID = xid.New().String()
		req.SendTime = now
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

func (cu *ComputeUnit) sendVectorShadowBufferAccesses(
	now sim.VTimeInSec,
) bool {
	if len(cu.shadowInFlightVectorMemAccess) > 0 {
		info := cu.shadowInFlightVectorMemAccess[0]
		if info.Read != nil {
			req := info.Read
			req.ID = sim.GetIDGenerator().Generate()
			req.SendTime = now
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
			req.SendTime = now
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

func (cu *ComputeUnit) sendInstFetchShadowBufferAccesses(
	now sim.VTimeInSec,
) bool {
	if len(cu.shadowInFlightInstFetch) > 0 {
		info := cu.shadowInFlightInstFetch[0]
		req := info.Req
		req.ID = xid.New().String()
		req.SendTime = now
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

	cu.ToACE = sim.NewLimitNumMsgPort(cu, 4, name+".ToACE")
	cu.toACESender = sim.NewBufferedSender(
		cu.ToACE, sim.NewBuffer(cu.Name()+".ToACESenderBuffer", 40960000))
	cu.ToInstMem = sim.NewLimitNumMsgPort(cu, 4, name+".ToInstMem")
	cu.ToScalarMem = sim.NewLimitNumMsgPort(cu, 4, name+".ToScalarMem")
	cu.ToVectorMem = sim.NewLimitNumMsgPort(cu, 4, name+".ToVectorMem")
	cu.ToCP = sim.NewLimitNumMsgPort(cu, 4, name+".ToCP")

	return cu
}
