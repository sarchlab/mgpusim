package wavefront

import (
	"github.com/sarchlab/akita/v5/timing"
)

// A WfCompletionEvent marks the completion of a wavefront. Events dispatch by
// handler ID in Akita v5: the compute unit registers its handler with the
// engine (engine.(timing.HandlerRegistrar).RegisterHandler) and passes the
// same handler ID when constructing the event.
type WfCompletionEvent struct {
	timing.EventBase
	Wf *Wavefront
}

// NewWfCompletionEvent returns a newly constructed WfCompletionEvent
func NewWfCompletionEvent(
	time timing.VTimeInPicoSec,
	handlerID string,
	wf *Wavefront,
) WfCompletionEvent {
	return WfCompletionEvent{
		EventBase: timing.MakeEventBase(time, handlerID),
		Wf:        wf,
	}
}
