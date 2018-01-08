package timing

import "gitlab.com/yaotsu/core"

// A WfCompleteEvent marks the completion of a wavefront
type WfCompleteEvent struct {
	*core.EventBase
	Wf *Wavefront
}

// NewWfCompleteEvent returns a newly constructed WfCompleteEvent
func NewWfCompleteEvent(time core.VTimeInSec, handler core.Handler,
	wf *Wavefront,
) *WfCompleteEvent {
	evt := new(WfCompleteEvent)
	evt.EventBase = core.NewEventBase(time, handler)
	evt.Wf = wf
	return evt
}
