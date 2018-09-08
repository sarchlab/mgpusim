package emu

import "gitlab.com/akita/akita"
import (
	"gitlab.com/akita/gcn3"
)

// WGCompleteEvent is an event that marks the completion of a work-group
type WGCompleteEvent struct {
	*akita.EventBase

	Req *gcn3.MapWGReq
}

// NewWGCompleteEvent returns a newly constructed WGCompleteEvent
func NewWGCompleteEvent(t akita.VTimeInSec, handler akita.Handler,
	req *gcn3.MapWGReq,
) *WGCompleteEvent {
	e := new(WGCompleteEvent)
	e.EventBase = akita.NewEventBase(t, handler)
	e.Req = req
	return e
}
