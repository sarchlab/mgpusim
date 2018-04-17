package timing

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/mem"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/util"
	"gitlab.com/yaotsu/gcn3"
)

// A ComputeUnit in the timing package provides a detailed and accurate
// simulation of a GCN3 ComputeUnit
type ComputeUnit struct {
	*core.ComponentBase

	WGMapper     WGMapper
	WfDispatcher WfDispatcher
	Decoder      emu.Decoder

	engine core.Engine
	Freq   util.Freq

	WfPools              []*WavefrontPool
	WfToDispatch         map[*kernels.Wavefront]*WfDispatchInfo
	wgToManagedWgMapping map[*kernels.WorkGroup]*WorkGroup
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

	InstMem   core.Component
	ScalarMem core.Component
	VectorMem core.Component
}

// NewComputeUnit returns a newly constructed compute unit
func NewComputeUnit(
	name string,
	engine core.Engine,
) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.ComponentBase = core.NewComponentBase(name)

	cu.engine = engine

	cu.WfToDispatch = make(map[*kernels.Wavefront]*WfDispatchInfo)
	cu.wgToManagedWgMapping = make(map[*kernels.WorkGroup]*WorkGroup)

	cu.AddPort("ToACE")
	cu.AddPort("ToInstMem")
	cu.AddPort("ToScalarMem")
	cu.AddPort("ToVectorMem")

	return cu
}

// Recv processes incoming requests
func (cu *ComputeUnit) Recv(req core.Req) *core.Error {
	util.ProcessReqAsEvent(req, cu.engine, cu.Freq)
	return nil
}

// Handle processes that events that are scheduled on the ComputeUnit
func (cu *ComputeUnit) Handle(evt core.Event) error {
	cu.InvokeHook(evt, cu, core.BeforeEvent, nil)
	defer cu.InvokeHook(evt, cu, core.AfterEvent, nil)

	switch evt := evt.(type) {
	case *gcn3.MapWGReq:
		return cu.handleMapWGReq(evt)
	case *gcn3.DispatchWfReq:
		return cu.handleDispatchWfReq(evt)
	case *WfDispatchCompletionEvent:
		return cu.handleWfDispatchCompletionEvent(evt)
	case *core.TickEvent:
		return cu.handleTickEvent(evt)
	case *WfCompletionEvent:
		return cu.handleWfCompletionEvent(evt)
	case *mem.AccessReq:
		return cu.handleMemAccessReq(evt)
	default:
		log.Panicf("Unable to process evevt of type %s",
			reflect.TypeOf(evt))
	}

	return nil
}

func (cu *ComputeUnit) handleMapWGReq(req *gcn3.MapWGReq) error {
	//log.Printf("%s map wg at %.12f\n", cu.Name(), req.Time())

	ok := false

	if len(cu.WfToDispatch) == 0 {
		ok = cu.WGMapper.MapWG(req)
	}

	if ok {
		cu.wrapWG(req.WG, req)
	}

	req.Ok = ok
	req.SwapSrcAndDst()
	req.SetSendTime(req.Time())
	err := cu.GetConnection("ToACE").Send(req)
	if err != nil {
		log.Panic(err)
	}

	return nil
}

func (cu *ComputeUnit) handleDispatchWfReq(req *gcn3.DispatchWfReq) error {
	wf := cu.wrapWf(req.Wf)
	cu.WfDispatcher.DispatchWf(wf, req)

	return nil
}

func (cu *ComputeUnit) handleWfDispatchCompletionEvent(
	evt *WfDispatchCompletionEvent,
) error {
	wf := evt.ManagedWf
	info := cu.WfToDispatch[wf.Wavefront]

	cu.WfPools[info.SIMDID].AddWf(wf)
	delete(cu.WfToDispatch, wf.Wavefront)
	wf.State = WfReady

	// Respond ACK
	req := evt.DispatchWfReq
	req.SwapSrcAndDst()
	req.SetSendTime(evt.Time())
	cu.GetConnection("ToACE").Send(req)

	if !cu.running {
		tick := core.NewTickEvent(cu.Freq.NextTick(evt.Time()), cu)
		cu.engine.Schedule(tick)
		cu.running = true
	}

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
	mesg := gcn3.NewWGFinishMesg(cu, dispatcher, now, wg.WorkGroup)

	err := cu.GetConnection("ToACE").Send(mesg)
	if err != nil {
		if !err.Recoverable {
			log.Fatal(err)
		} else {
			evt.SetTime(cu.Freq.NoEarlierThan(err.EarliestRetry))
			cu.engine.Schedule(evt)
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

func (cu *ComputeUnit) handleTickEvent(evt *core.TickEvent) error {
	now := evt.Time()

	cu.BranchUnit.Run(now)

	cu.ScalarUnit.Run(now)
	cu.ScalarDecoder.Run(now)

	for _, simdUnit := range cu.SIMDUnit {
		simdUnit.Run(now)
	}
	cu.VectorDecoder.Run(now)

	cu.Scheduler.EvaluateInternalInst(now)
	cu.Scheduler.DoIssue(now)
	cu.Scheduler.DoFetch(now)

	if cu.running {
		evt.SetTime(cu.Freq.NextTick(evt.Time()))
		cu.engine.Schedule(evt)
	}

	return nil
}

func (cu *ComputeUnit) wrapWG(
	raw *kernels.WorkGroup,
	req *gcn3.MapWGReq,
) *WorkGroup {
	wg := NewWorkGroup(raw, req)
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

func (cu *ComputeUnit) handleMemAccessReq(req *mem.AccessReq) error {
	info := req.Info.(*MemAccessInfo)
	if info.Action == MemAccessInstFetch {
		return cu.handleFetchReturn(req)
	}
	return nil
}

func (cu *ComputeUnit) handleFetchReturn(req *mem.AccessReq) error {
	info := req.Info.(*MemAccessInfo)
	wf := info.Wf
	wf.State = WfFetched
	wf.LastFetchTime = req.Time()

	inst, err := cu.Decoder.Decode(req.Buf)
	if err != nil {
		return err
	}
	managedInst := wf.ManagedInst()
	managedInst.Inst = inst
	wf.PC += uint64(managedInst.ByteSize)

	// log.Printf("%f: %s\n", req.Time(), wf.Inst.String())
	// wf.State = WfReady

	cu.InvokeHook(wf, cu, core.Any, &InstHookInfo{req.Time(), "FetchDone"})

	return nil
}
