package emu

import "gitlab.com/yaotsu/core"
import "gitlab.com/yaotsu/gcn3/kernels"

// WGCompleteEvent is an event that marks the completion of a work-group
type WGCompleteEvent struct {
	*core.EventBase

	WG *kernels.WorkGroup
}

// NewWGCompleteEvent returns a newly constructed WGCompleteEvent
func NewWGCompleteEvent(t core.VTimeInSec, handler core.Handler,
	wg *kernels.WorkGroup,
) *WGCompleteEvent {
	e := new(WGCompleteEvent)
	e.EventBase = core.NewEventBase(t, handler)
	e.WG = wg
	return e
}
