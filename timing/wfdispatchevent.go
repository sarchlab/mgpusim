package timing

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
)

// WfDispatchCompletionEvent marks the completion of a wavefront dispatching
type WfDispatchEvent struct {
	*core.EventBase

	ManagedWf  *Wavefront
	IsLastInWG bool
	MapWGReq   *gcn3.MapWGReq
}

// NewWfDispatchEvent creates a new WfDispatchCompletionEvent
func NewWfDispatchEvent(
	t core.VTimeInSec,
	handler core.Handler,
	Wf *Wavefront,
) *WfDispatchEvent {
	evt := new(WfDispatchEvent)
	evt.EventBase = core.NewEventBase(t, handler)
	evt.ManagedWf = Wf
	return evt
}
