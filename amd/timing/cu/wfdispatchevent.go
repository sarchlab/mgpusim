package cu

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// WfDispatchEvent is the event that the dispatcher dispatches a wavefront.
// The compute unit registers its handler for this event under the ID
// "<comp name>.WfDispatch"; pass that ID as handlerID when constructing the
// event.
type WfDispatchEvent struct {
	timing.EventBase

	Handle     waveHandle
	IsLastInWG bool
	MapWGReq   messaging.Msg
}

// NewWfDispatchEvent creates a new WfDispatchEvent.
func NewWfDispatchEvent(
	id uint64,
	t timing.VTimeInPicoSec,
	handlerID string,
	handle waveHandle,
) WfDispatchEvent {
	return WfDispatchEvent{
		EventBase: timing.MakeEventBase(id, t, handlerID),
		Handle:    handle,
	}
}

func init() { timing.RegisterEvent(WfDispatchEvent{}) }
