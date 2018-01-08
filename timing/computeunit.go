package timing

import (
	"log"
	"reflect"

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

	WfToDispatch []*WfDispatchInfo
	running      bool
}

// NewComputeUnit returns a newly constructed compute unit
func NewComputeUnit(name string, engine core.Engine) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.ComponentBase = core.NewComponentBase(name)

	cu.engine = engine

	cu.WfToDispatch = make([]*WfDispatchInfo, 0)

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
	case *core.TickEvent:
		return cu.handleTickEvent(evt)
	case *WfDispatchCompletionEvent:
		return cu.handleWfDispatchCompletionEvent(evt)
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
	cu.WfDispatcher.DispatchWf(req)
	return nil
}

func (cu *ComputeUnit) handleWfDispatchCompletionEvent(
	evt *WfDispatchCompletionEvent,
) error {
	if !cu.running {
		tick := core.NewTickEvent(cu.Freq.NextTick(evt.Time()), cu)
		cu.engine.Schedule(tick)
	}
	return nil
}

func (cu *ComputeUnit) handleWfCompleteEvent(evt *WfCompleteEvent) error {
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

	if len(s.RunningWGs) == 0 {
		s.running = false
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

func (cu *ComputeUnit) handleTickEvent(evt *core.TickEvent) error {
	return nil
}
