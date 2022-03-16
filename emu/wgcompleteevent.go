package emu

import (
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/mgpusim/v2/protocol"
)

// WGCompleteEvent is an event that marks the completion of a work-group
type WGCompleteEvent struct {
	*sim.EventBase

	Req *protocol.MapWGReq
}

// NewWGCompleteEvent returns a newly constructed WGCompleteEvent
func NewWGCompleteEvent(t sim.VTimeInSec, handler sim.Handler,
	req *protocol.MapWGReq,
) *WGCompleteEvent {
	e := new(WGCompleteEvent)
	e.EventBase = sim.NewEventBase(t, handler)
	e.Req = req
	return e
}
