package emu

import "gitlab.com/yaotsu/core"
import (
	"gitlab.com/yaotsu/gcn3"
)

// WGCompleteEvent is an event that marks the completion of a work-group
type WGCompleteEvent struct {
	*core.EventBase

	Req *gcn3.MapWGReq
}

// NewWGCompleteEvent returns a newly constructed WGCompleteEvent
func NewWGCompleteEvent(t core.VTimeInSec, handler core.Handler,
	req *gcn3.MapWGReq,
) *WGCompleteEvent {
	e := new(WGCompleteEvent)
	e.EventBase = core.NewEventBase(t, handler)
	e.Req = req
	return e
}
