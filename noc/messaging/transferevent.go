package messaging

import "github.com/sarchlab/akita/v3/sim"

// A TransferEvent is an event that marks that a message completes transfer.
type TransferEvent struct {
	*sim.EventBase
	msg sim.Msg
	vc  int
}

// NewTransferEvent creates a new TransferEvent.
func NewTransferEvent(
	time sim.VTimeInSec,
	handler sim.Handler,
	msg sim.Msg,
	vc int,
) *TransferEvent {
	evt := new(TransferEvent)
	evt.EventBase = sim.NewEventBase(time, handler)
	evt.msg = msg
	evt.vc = vc
	return evt
}
