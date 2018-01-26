package timing

import (
	"log"
	"reflect"

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

	engine core.Engine
	Freq   util.Freq

	WfToDispatch         map[*kernels.Wavefront]*WfDispatchInfo
	wgToManagedWgMapping map[*kernels.WorkGroup]*WorkGroup
	running              bool

	Scheduler       *Scheduler
	BranchUnit      CUComponent
	VectorMemDecode CUComponent
	VectorMemUnit   CUComponent
	ScalarDecode    CUComponent
	VectorDecode    CUComponent
	LDSDecode       CUComponent
	ScalarUnit      CUComponent
	SIMDUnit        []CUComponent
	WfPools         []*WavefrontPool
	LDSUnit         CUComponent

	InstMem core.Component
}

// NewComputeUnit returns a newly constructed compute unit
func NewComputeUnit(name string, engine core.Engine) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.ComponentBase = core.NewComponentBase(name)

	cu.engine = engine

	cu.WfToDispatch = make(map[*kernels.Wavefront]*WfDispatchInfo)
	cu.wgToManagedWgMapping = make(map[*kernels.WorkGroup]*WorkGroup)

	cu.AddPort("ToACE")
	cu.AddPort("ToInstMem")
	cu.AddPort("ToDataMem")

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
	}

	// This is temporary code
	// wfCompletionEvent := NewWfCompletionEvent(
	// 	cu.Freq.NCyclesLater(10000, evt.Time()),
	// 	cu, evt.ManagedWf)
	// cu.engine.Schedule(wfCompletionEvent)

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
			// delete(s.RunningWGs, wf.WG)
		}
	}

	return nil
}

func (cu *ComputeUnit) clearWGResource(wg *WorkGroup) {
	cu.WGMapper.UnmapWG(wg)
	//for _, wf := range wg.Wfs {
	//	wfPool := s.WfPools[wf.SIMDID]
	//	wfPool.RemoveWf(wf)
	//}
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

func (cu *ComputeUnit) handleTickEvent(evt *core.TickEvent) error {
	// cu.Scheduler.DoIssue()
	cu.Scheduler.DoFetch(evt.Time())
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
	wf := new(Wavefront)
	wf.Wavefront = raw
	wg := cu.wgToManagedWgMapping[raw.WG]
	wg.Wfs = append(wg.Wfs, wf)
	wf.WG = wg
	return wf
}

func (cu *ComputeUnit) handleMemAccessReq(req *mem.AccessReq) error {
	return nil
}
