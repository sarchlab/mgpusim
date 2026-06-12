package wavefront

import (
	"github.com/sarchlab/akita/v5/sim"
)

// A WfCompletionEvent marks the completion of a wavefront
type WfCompletionEvent struct {
	*sim.EventBase
	Wf *Wavefront
}

// NewWfCompletionEvent returns a newly constructed WfCompleteEvent
func NewWfCompletionEvent(
	time sim.VTimeInSec,
	handlerName string,
	wf *Wavefront,
) *WfCompletionEvent {
	evt := new(WfCompletionEvent)
	evt.EventBase = sim.NewEventBase(time, handlerName)
	evt.Wf = wf
	return evt
}
