package cu

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v3/protocol"
	"github.com/sarchlab/mgpusim/v3/timing/wavefront"
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
