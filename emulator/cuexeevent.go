package emulator

import (
	"gitlab.com/yaotsu/core/event"
)

// A CUExeEvent is a event that let the ComputeUnit to execute the next
// instruciton
type CUExeEvent struct {
	*event.BasicEvent
}

// NewCUExeEvent creates and returns a new CUExeEvent
func NewCUExeEvent() *CUExeEvent {
	e := new(CUExeEvent)
	e.BasicEvent = event.NewBasicEvent()
	return e
}
