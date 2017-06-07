package emu

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
)

// A ComputeUnit in the emu package is a component that omit the pipeline design
// but can still run the GCN3 instructions.
//
//     ToDispatcher <=> The port that connect the CU with the dispatcher
//
type ComputeUnit struct {
	*core.ComponentBase

	engine core.Engine
	Freq   core.Freq

	running *gcn3.MapWGReq
}

// NewComputeUnit creates a new ComputeUnit with the given name
func NewComputeUnit(name string, engine core.Engine) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.ComponentBase = core.NewComponentBase(name)

	cu.engine = engine

	cu.AddPort("ToDispatcher")

	return cu
}

// Recv accepts requests from other components
func (cu *ComputeUnit) Recv(req core.Req) *core.Error {
	switch req := req.(type) {
	case *gcn3.MapWGReq:
		return cu.processMapWGReq(req)
	case *gcn3.DispatchWfReq:
		return cu.processDispatchWfReq(req)
	default:
		log.Panicf("cannot process req %s", reflect.TypeOf(req))
	}
	return nil
}

func (cu *ComputeUnit) processMapWGReq(req *gcn3.MapWGReq) *core.Error {
	if cu.running != nil {
		req.Ok = false
	} else {
		req.Ok = true
		cu.running = req

		log.Printf("WG mapped")

		evt := NewWGCompleteEvent(cu.Freq.NCyclesLater(3000, req.RecvTime()),
			cu, req.WG)
		cu.engine.Schedule(evt)
	}

	req.SwapSrcAndDst()
	req.SetSendTime(cu.Freq.HalfTick(req.RecvTime()))
	deferredSend := core.NewDeferredSend(req)
	cu.engine.Schedule(deferredSend)

	return nil
}

func (cu *ComputeUnit) processDispatchWfReq(req core.Req) *core.Error {
	// This function is itentionally left blank
	// The emulator does not need to deal with DispatchWfReq since the
	// execution will start when it processes the MapWGReq
	return nil
}

// Handle defines the behavior on event scheduled on the ComputeUnit
func (cu *ComputeUnit) Handle(evt core.Event) error {
	switch evt := evt.(type) {
	case *WGCompleteEvent:
		return cu.handleWGCompleteEvent(evt)
	case *core.DeferredSend:
		return cu.handleDeferredSend(evt)
	default:
		log.Panicf("cannot handle event %s", reflect.TypeOf(evt))
	}
	return nil
}

func (cu *ComputeUnit) handleWGCompleteEvent(evt *WGCompleteEvent) error {
	req := gcn3.NewWGFinishMesg(cu, cu.running.Dst(), evt.Time(), cu.running.WG)
	cu.GetConnection("ToDispatcher").Send(req)
	cu.running = nil
	return nil
}

func (cu *ComputeUnit) handleDeferredSend(evt *core.DeferredSend) error {
	req := evt.Req
	cu.GetConnection("ToDispatcher").Send(req)
	return nil
}
