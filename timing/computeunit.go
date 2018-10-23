package timing

import (
	"log"
	"reflect"

	"gitlab.com/akita/gcn3/emu"
	"gitlab.com/akita/gcn3/kernels"
	"gitlab.com/akita/mem"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/mem/cache"
)

// A ComputeUnit in the timing package provides a detailed and accurate
// simulation of a GCN3 ComputeUnit
type ComputeUnit struct {
	*akita.ComponentBase
	ticker *akita.Ticker

	WGMapper     WGMapper
	WfDispatcher WfDispatcher
	Decoder      emu.Decoder

	engine akita.Engine
	Freq   akita.Freq

	WfPools              []*WavefrontPool
	WfToDispatch         map[*kernels.Wavefront]*WfDispatchInfo
	wgToManagedWgMapping map[*kernels.WorkGroup]*WorkGroup
	inFlightMemAccess    map[string]*MemAccessInfo
	running              bool

	Scheduler        *Scheduler
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

	InstMem          *akita.Port
	ScalarMem        *akita.Port
	VectorMemModules cache.LowModuleFinder

	ToACE       *akita.Port
	ToInstMem   *akita.Port
	ToScalarMem *akita.Port
	ToVectorMem *akita.Port
}

func (cu *ComputeUnit) NotifyRecv(now akita.VTimeInSec, port *akita.Port) {
	req := port.Retrieve(now)
	akita.ProcessReqAsEvent(req, cu.engine, cu.Freq)
}

func (cu *ComputeUnit) NotifyPortFree(now akita.VTimeInSec, port *akita.Port) {
	//panic("implement me")
}

// Handle processes that events that are scheduled on the ComputeUnit
func (cu *ComputeUnit) Handle(evt akita.Event) error {
	cu.Lock()
	defer cu.Unlock()

	cu.InvokeHook(evt, cu, akita.BeforeEventHookPos, nil)
	defer cu.InvokeHook(evt, cu, akita.AfterEventHookPos, nil)

	switch evt := evt.(type) {
	case *gcn3.MapWGReq:
		return cu.handleMapWGReq(evt)
	case *WfDispatchEvent:
		return cu.handleWfDispatchEvent(evt)
	case *akita.TickEvent:
		return cu.handleTickEvent(evt)
	case *WfCompletionEvent:
		return cu.handleWfCompletionEvent(evt)
	case *mem.DataReadyRsp:
		return cu.handleDataReadyRsp(evt)
	case *mem.DoneRsp:
		return cu.handleMemDoneRsp(evt)
	default:
		log.Panicf("Unable to process evevt of type %s",
			reflect.TypeOf(evt))
	}

	return nil
}

func (cu *ComputeUnit) handleMapWGReq(req *gcn3.MapWGReq) error {
	//log.Printf("%s map wg at %.12f\n", cu.Name(), req.Time())
	now := req.Time()

	ok := false

	if len(cu.WfToDispatch) == 0 {
		ok = cu.WGMapper.MapWG(req)
	}

	wfs := make([]*Wavefront, 0)
	if ok {
		cu.wrapWG(req.WG, req)
		for _, wf := range req.WG.Wavefronts {
			managedWf := cu.wrapWf(wf)
			wfs = append(wfs, managedWf)
		}

		for i, wf := range wfs {
			evt := NewWfDispatchEvent(cu.Freq.NCyclesLater(i, now), cu, wf)
			cu.engine.Schedule(evt)
		}

		lastEventCycle := 4
		if len(wfs) > 4 {
			lastEventCycle = len(wfs)
		}
		evt := NewWfDispatchEvent(cu.Freq.NCyclesLater(lastEventCycle, now), cu, nil)
		evt.MapWGReq = req
		evt.IsLastInWG = true
		cu.engine.Schedule(evt)

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
		wf.State = WfReady
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
	cu.ticker.TickLater(evt.Time())

	return nil
}

func (cu *ComputeUnit) handleWfCompletionEvent(evt *WfCompletionEvent) error {
	wf := evt.Wf
	wg := wf.WG
	wf.State = WfCompleted

	if cu.isAllWfInWGCompleted(wg) {
		ok := cu.sendWGCompletionMessage(evt, wg)
		if ok {
			cu.clearWGResource(wg)
		}

		if !cu.hasMoreWfsToRun() {
			cu.running = false
		}
	}

	return nil
}

func (cu *ComputeUnit) clearWGResource(wg *WorkGroup) {
	cu.WGMapper.UnmapWG(wg)
	for _, wf := range wg.Wfs {
		wfPool := cu.WfPools[wf.SIMDID]
		wfPool.RemoveWf(wf)
	}
}

func (cu *ComputeUnit) isAllWfInWGCompleted(wg *WorkGroup) bool {
	for _, wf := range wg.Wfs {
		if wf.State != WfCompleted {
			return false
		}
	}
	return true
}

func (cu *ComputeUnit) sendWGCompletionMessage(
	evt *WfCompletionEvent,
	wg *WorkGroup,
) bool {
	mapReq := wg.MapReq
	dispatcher := mapReq.Dst() // This is dst since the mapReq has been sent back already
	now := evt.Time()
	mesg := gcn3.NewWGFinishMesg(cu.ToACE, dispatcher, now, wg.WorkGroup)

	err := cu.ToACE.Send(mesg)
	if err != nil {
		newEvent := NewWfCompletionEvent(cu.Freq.NextTick(now), cu, evt.Wf)
		cu.engine.Schedule(newEvent)
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

func (cu *ComputeUnit) handleTickEvent(evt *akita.TickEvent) error {
	now := evt.Time()

	cu.BranchUnit.Run(now)

	cu.ScalarUnit.Run(now)
	cu.ScalarDecoder.Run(now)

	for _, simdUnit := range cu.SIMDUnit {
		simdUnit.Run(now)
	}
	cu.VectorDecoder.Run(now)

	cu.LDSUnit.Run(now)
	cu.LDSDecoder.Run(now)

	cu.VectorMemUnit.Run(now)
	cu.VectorMemDecoder.Run(now)

	cu.Scheduler.Run(now)

	if cu.running {
		cu.ticker.TickLater(now)
	}

	return nil
}

func (cu *ComputeUnit) wrapWG(
	raw *kernels.WorkGroup,
	req *gcn3.MapWGReq,
) *WorkGroup {
	wg := NewWorkGroup(raw, req)

	lds := make([]byte, wg.CodeObject().WGGroupSegmentByteSize)
	wg.LDS = lds

	cu.wgToManagedWgMapping[raw] = wg
	return wg
}

func (cu *ComputeUnit) wrapWf(raw *kernels.Wavefront) *Wavefront {
	wf := NewWavefront(raw)
	wg := cu.wgToManagedWgMapping[raw.WG]
	wg.Wfs = append(wg.Wfs, wf)
	wf.WG = wg
	wf.CodeObject = wf.WG.Grid.CodeObject
	wf.Packet = wf.WG.Grid.Packet
	wf.PacketAddress = wf.WG.Grid.PacketAddress
	return wf
}

func (cu *ComputeUnit) handleDataReadyRsp(rsp *mem.DataReadyRsp) error {
	info, found := cu.inFlightMemAccess[rsp.RespondTo]
	if !found {
		log.Panic("memory access request not sent from the unit")
	}

	switch info.Action {
	case MemAccessInstFetch:
		return cu.handleFetchReturn(rsp, info)
	case MemAccessScalarDataLoad:
		return cu.handleScalarDataLoadReturn(rsp, info)
	case MemAccessVectorDataLoad:
		return cu.handleVectorDataLoadReturn(rsp, info)
	}

	return nil
}

func (cu *ComputeUnit) handleMemDoneRsp(rsp *mem.DoneRsp) error {
	info, found := cu.inFlightMemAccess[rsp.RespondTo]
	if !found {
		log.Panic("memory access request not sent from the unit")
	}

	return cu.handleVectorDataStoreRsp(rsp, info)
}

func (cu *ComputeUnit) handleFetchReturn(rsp *mem.DataReadyRsp, info *MemAccessInfo) error {
	now := rsp.Time()
	wf := info.Wf
	addr := info.Address
	delete(cu.inFlightMemAccess, rsp.RespondTo)

	if addr == wf.InstBufferStartPC+uint64(len(wf.InstBuffer)) {
		wf.InstBuffer = append(wf.InstBuffer, rsp.Data...)
	}

	wf.IsFetching = false
	wf.LastFetchTime = now

	return nil
}

func (cu *ComputeUnit) handleScalarDataLoadReturn(rsp *mem.DataReadyRsp, info *MemAccessInfo) error {
	wf := info.Wf

	access := new(RegisterAccess)
	access.WaveOffset = wf.SRegOffset
	access.Reg = info.Dst
	access.RegCount = int(len(rsp.Data) / 4)
	access.Data = rsp.Data
	cu.SRegFile.Write(access)

	wf.OutstandingScalarMemAccess -= 1
	delete(cu.inFlightMemAccess, rsp.RespondTo)

	cu.InvokeHook(wf, cu, akita.AnyHookPos, &InstHookInfo{rsp.Time(), info.Inst, "Completed"})

	return nil
}

func (cu *ComputeUnit) handleVectorDataLoadReturn(
	rsp *mem.DataReadyRsp,
	info *MemAccessInfo,
) error {
	wf := info.Wf
	inst := info.Inst

	for i := 0; i < 64; i++ {
		addr := info.PreCoalescedAddrs[i]
		addrCacheLineID := addr & 0xffffffffffffffc0
		addrCacheLineOffset := addr & 0x000000000000003f
		if addrCacheLineID != info.Address {
			continue
		}

		access := new(RegisterAccess)
		access.WaveOffset = wf.VRegOffset
		access.Reg = info.Dst
		access.RegCount = info.RegCount
		access.LaneID = i
		if inst.FormatType == insts.FLAT && inst.Opcode == 16 { // FLAT_LOAD_UBYTE
			access.Data = insts.Uint32ToBytes(uint32(
				rsp.Data[addrCacheLineOffset]))
		} else {
			access.Data = rsp.Data[addrCacheLineOffset : addrCacheLineOffset+uint64(4*info.RegCount)]
		}

		cu.VRegFile[wf.SIMDID].Write(access)
	}

	info.ReturnedReqs += 1
	if info.ReturnedReqs == info.TotalReqs {
		wf.OutstandingVectorMemAccess--
		cu.InvokeHook(wf, cu, akita.AnyHookPos, &InstHookInfo{rsp.Time(), info.Inst, "Completed"})
	}

	delete(cu.inFlightMemAccess, rsp.RespondTo)

	return nil
}

func (cu *ComputeUnit) handleVectorDataStoreRsp(rsp *mem.DoneRsp, info *MemAccessInfo) error {
	wf := info.Wf

	info.ReturnedReqs += 1
	if info.ReturnedReqs == info.TotalReqs {
		wf.OutstandingVectorMemAccess--
		cu.InvokeHook(wf, cu, akita.AnyHookPos, &InstHookInfo{rsp.Time(), info.Inst, "Completed"})
	}
	delete(cu.inFlightMemAccess, rsp.RespondTo)
	return nil
}

// NewComputeUnit returns a newly constructed compute unit
func NewComputeUnit(
	name string,
	engine akita.Engine,
) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.ComponentBase = akita.NewComponentBase(name)

	cu.engine = engine
	cu.Freq = 1 * akita.GHz
	cu.ticker = akita.NewTicker(cu, engine, cu.Freq)

	cu.WfToDispatch = make(map[*kernels.Wavefront]*WfDispatchInfo)
	cu.wgToManagedWgMapping = make(map[*kernels.WorkGroup]*WorkGroup)
	cu.inFlightMemAccess = make(map[string]*MemAccessInfo)

	cu.ToACE = akita.NewPort(cu)
	cu.ToInstMem = akita.NewPort(cu)
	cu.ToScalarMem = akita.NewPort(cu)
	cu.ToVectorMem = akita.NewPort(cu)

	return cu
}
