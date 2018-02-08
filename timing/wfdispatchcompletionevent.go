package timing

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
)

// WfDispatchCompletionEvent marks the completion of a wavefront dispatching
type WfDispatchCompletionEvent struct {
	*core.EventBase

	ManagedWf     *Wavefront
	DispatchWfReq *gcn3.DispatchWfReq
}

// NewWfDispatchCompletionEvent creates a new WfDispatchCompletionEvent
func NewWfDispatchCompletionEvent(
	t core.VTimeInSec,
	handler core.Handler,
	Wf *Wavefront,
) *WfDispatchCompletionEvent {
	evt := new(WfDispatchCompletionEvent)
	evt.EventBase = core.NewEventBase(t, handler)
	evt.ManagedWf = Wf
	return evt
}
