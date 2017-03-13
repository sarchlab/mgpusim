package emu

import "gitlab.com/yaotsu/core"

// A CUExeEvent is a event that let the ComputeUnit to execute the next
// instruciton
type CUExeEvent struct {
	*core.BasicEvent
}

// NewCUExeEvent creates and returns a new CUExeEvent
func NewCUExeEvent() *CUExeEvent {
	e := new(CUExeEvent)
	e.BasicEvent = core.NewBasicEvent()
	return e
}
