package cu

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/timing"
)

// DispatchingState represents to progress of a wavefront dispatching
type DispatchingState int

// A list of possible dispatching states
const (
	NotStarted  DispatchingState = iota
	Initialized                  // Inserted in the wavefront pool,
	SRegSet                      // Done with sending s reg write request
	VRegSet                      // Done with sending v reg write request
	Completed                    // All the register writing has completed
)

// DispatchWfEvent requires the scheduler shart to schedule for the event.
type DispatchWfEvent struct {
	*core.BasicEvent

	Req *timing.DispatchWfReq

	Initialized    bool
	SRegInitialize bool
}

// NewDispatchWfEvent returns a newly created DispatchWfEvent
func NewDispatchWfEvent(
	handler core.Handler,
	time core.VTimeInSec,
	req *timing.DispatchWfReq,
) *DispatchWfEvent {
	e := new(DispatchWfEvent)
	e.BasicEvent = core.NewBasicEvent()
	e.SetHandler(handler)
	e.SetTime(time)
	e.Req = req
	return e
}

// A WfDispatcher initiaize wavefronts
type WfDispatcher interface {
	DispatchWf(evt *DispatchWfEvent)
}

// A WfDispatcherImpl will register the wavefront in wavefront pool and
// initialize all the registers
type WfDispatcherImpl struct {
	scheduler *Scheduler
}
