package cu

import (
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// WfDispatchEvent is the event that the dispatcher dispatches a wavefront.
// The compute unit registers its handler for this event under the ID
// "<comp name>.WfDispatch"; pass that ID as handlerID when constructing the
// event.
type WfDispatchEvent struct {
	timing.EventBase

	ManagedWf  *wavefront.Wavefront
	IsLastInWG bool
	MapWGReq   protocol.MapWGReq
}

// NewWfDispatchEvent creates a new WfDispatchEvent.
func NewWfDispatchEvent(
	t timing.VTimeInPicoSec,
	handlerID string,
	wf *wavefront.Wavefront,
) WfDispatchEvent {
	return WfDispatchEvent{
		EventBase: timing.MakeEventBase(t, handlerID),
		ManagedWf: wf,
	}
}
