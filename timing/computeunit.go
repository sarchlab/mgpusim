package timing

import (
	"log"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/util"
	"gitlab.com/yaotsu/gcn3"
)

// A ComputeUnit in the timing package provides a detailed and accurate
// simulation of a GCN3 ComputeUnit
type ComputeUnit struct {
	*core.ComponentBase

	wgMapper WGMapper

	engine core.Engine
	Freq   util.Freq

	WfToDispatch []*WfDispatchInfo
}

// NewComputeUnit returns a newly constructed compute unit
func NewComputeUnit(name string) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.ComponentBase = core.NewComponentBase(name)

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
	}

	return nil
}

func (cu *ComputeUnit) handleMapWGReq(req *gcn3.MapWGReq) *core.Error {
	ok := false

	if len(cu.WfToDispatch) == 0 {
		ok = cu.wgMapper.MapWG(req)
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

func (cu *ComputeUnit) handleDispatchWfReq(req *gcn3.DispatchWfReq) *core.Error {
	return nil
}

func (cu *ComputeUnit) handleTickEvent(evt *core.TickEvent) error {
	return nil
}
