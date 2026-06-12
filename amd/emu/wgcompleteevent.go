package emu

import (
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v4/amd/protocol"
)

// WGCompleteEvent is an event that marks the completion of a work-group
type WGCompleteEvent struct {
	*sim.EventBase

	Req *protocol.MapWGReq
}

// NewWGCompleteEvent returns a newly constructed WGCompleteEvent
func NewWGCompleteEvent(t sim.VTimeInSec, handlerName string,
	req *protocol.MapWGReq,
) *WGCompleteEvent {
	e := new(WGCompleteEvent)
	e.EventBase = sim.NewEventBase(t, handlerName)
	e.Req = req
	return e
}
