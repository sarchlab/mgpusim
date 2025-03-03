package wavefront

import (
	"github.com/sarchlab/akita/v3/sim"
	// "gitlab.com/akita/mgpusim/v3/timing/wavefront"
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
