package wavefront

import (
	"github.com/sarchlab/akita/v5/sim/timing"
)

// A WfCompletionEvent marks the completion of a wavefront. Events dispatch by
// handler ID in Akita v5: the compute unit registers its handler with the
// engine (engine.(timing.HandlerRegistrar).RegisterHandler) and passes the
// same handler ID when constructing the event.
type WfCompletionEvent struct {
	timing.EventBase
	// WavefrontID is the receiving CU's generation-bearing local slot handle.
	WavefrontID uint64
}

// NewWfCompletionEvent returns a newly constructed WfCompletionEvent
func NewWfCompletionEvent(
	id uint64,
	time timing.VTimeInPicoSec,
	handlerID string,
	wfID uint64,
) WfCompletionEvent {
	return WfCompletionEvent{
		EventBase:   timing.MakeEventBase(id, time, handlerID),
		WavefrontID: wfID,
	}
}

func init() { timing.RegisterEvent(WfCompletionEvent{}) }
