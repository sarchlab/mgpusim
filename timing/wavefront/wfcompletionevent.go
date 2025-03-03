package wavefront

import (
	"github.com/sarchlab/akita/v4/sim"
)

// A WfCompletionEvent marks the completion of a wavefront
type WfCompletionEvent struct {
	*sim.EventBase
	Wf *Wavefront
}

// NewWfCompletionEvent returns a newly constructed WfCompleteEvent
func NewWfCompletionEvent(
	time sim.VTimeInSec,
	handler sim.Handler,
	wf *Wavefront,
) *WfCompletionEvent {
	evt := new(WfCompletionEvent)
	evt.EventBase = sim.NewEventBase(time, handler)
	evt.Wf = wf
	return evt
}
