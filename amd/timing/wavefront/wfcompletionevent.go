package wavefront

import (
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Handle identifies a generation-bearing wave slot within one CU. It is not a
// kernel/global wave UID. Zero is invalid.
type Handle uint64

// A WfCompletionEvent marks the completion of a wavefront. Events dispatch by
// handler ID in Akita v5: the compute unit registers its handler with the
// engine (engine.(timing.HandlerRegistrar).RegisterHandler) and passes the
// same handler ID when constructing the event.
type WfCompletionEvent struct {
	timing.EventBase
	// Handle is the receiving CU's generation-bearing local slot handle.
	Handle Handle
}

// NewWfCompletionEvent returns a newly constructed WfCompletionEvent
func NewWfCompletionEvent(
	id uint64,
	time timing.VTimeInPicoSec,
	handlerID string,
	handle Handle,
) WfCompletionEvent {
	return WfCompletionEvent{
		EventBase: timing.MakeEventBase(id, time, handlerID),
		Handle:    handle,
	}
}

func init() { timing.RegisterEvent(WfCompletionEvent{}) }
