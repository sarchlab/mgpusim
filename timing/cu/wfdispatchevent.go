package cu

import (
	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/mgpusim/v2/protocol"
	"gitlab.com/akita/mgpusim/v2/timing/wavefront"
)

// WfDispatchEvent is the event that the dispatcher dispatches a wavefront
type WfDispatchEvent struct {
	*sim.EventBase

	ManagedWf  *wavefront.Wavefront
	IsLastInWG bool
	MapWGReq   *protocol.MapWGReq
}

// NewWfDispatchEvent creates a new WfDispatchCompletionEvent
func NewWfDispatchEvent(
	t sim.VTimeInSec,
	handler sim.Handler,
	Wf *wavefront.Wavefront,
) *WfDispatchEvent {
	evt := new(WfDispatchEvent)
	evt.EventBase = sim.NewEventBase(t, handler)
	evt.ManagedWf = Wf
	return evt
}
