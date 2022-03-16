package cu

import (
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/mgpusim/v2/timing/wavefront"
)

// A WfCompletionEvent marks the completion of a wavefront
type WfCompletionEvent struct {
	*sim.EventBase
	Wf *wavefront.Wavefront
}

// NewWfCompletionEvent returns a newly constructed WfCompleteEvent
func NewWfCompletionEvent(
	time sim.VTimeInSec,
	handler sim.Handler,
	wf *wavefront.Wavefront,
) *WfCompletionEvent {
	evt := new(WfCompletionEvent)
	evt.EventBase = sim.NewEventBase(time, handler)
	evt.Wf = wf
	return evt
}
