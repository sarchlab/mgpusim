package timing

import (
	"gitlab.com/akita/akita"
)

// A WfCompletionEvent marks the completion of a wavefront
type WfCompletionEvent struct {
	*akita.EventBase
	Wf *Wavefront
}

// NewWfCompletionEvent returns a newly constructed WfCompleteEvent
func NewWfCompletionEvent(time akita.VTimeInSec, handler akita.Handler,
	wf *Wavefront,
) *WfCompletionEvent {
	evt := new(WfCompletionEvent)
	evt.EventBase = akita.NewEventBase(time, handler)
	evt.Wf = wf
	return evt
}
