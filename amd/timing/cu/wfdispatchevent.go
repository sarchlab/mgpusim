package cu

import (
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v4/amd/protocol"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
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
	handlerName string,
	Wf *wavefront.Wavefront,
) *WfDispatchEvent {
	evt := new(WfDispatchEvent)
	evt.EventBase = sim.NewEventBase(t, handlerName)
	evt.ManagedWf = Wf
	return evt
}
