package timing

import (
	"gitlab.com/yaotsu/core"
)

// A WfCompletionEvent marks the completion of a wavefront
type WfCompletionEvent struct {
	*core.EventBase
	Wf *Wavefront
}

// NewWfCompletionEvent returns a newly constructed WfCompleteEvent
func NewWfCompletionEvent(time core.VTimeInSec, handler core.Handler,
	wf *Wavefront,
) *WfCompletionEvent {
	evt := new(WfCompletionEvent)
	evt.EventBase = core.NewEventBase(time, handler)
	evt.Wf = wf
	return evt
}
