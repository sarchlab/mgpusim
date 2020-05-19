package cu

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/timing/wavefront"
)

// A WfCompletionEvent marks the completion of a wavefront
type WfCompletionEvent struct {
	*akita.EventBase
	Wf *wavefront.Wavefront
}

// NewWfCompletionEvent returns a newly constructed WfCompleteEvent
func NewWfCompletionEvent(
	time akita.VTimeInSec,
	handler akita.Handler,
	wf *wavefront.Wavefront,
) *WfCompletionEvent {
	evt := new(WfCompletionEvent)
	evt.EventBase = akita.NewEventBase(time, handler)
	evt.Wf = wf
	return evt
}
