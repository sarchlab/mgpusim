package emu

import "gitlab.com/yaotsu/core"

// ContinueInstFunc defines the function that continues the execution of an
// instruction
type ContinueInstFunc func(*ContinueInstEvent) error

// ContinueInstEvent is an event that requests the ComputeUnit to pickup a
// unfinished instruction and continue to execute
type ContinueInstEvent struct {
	*core.BasicEvent
	Info             interface{}
	ContinueFunction ContinueInstFunc
}

// NewContinueInstEvent creates a new ContinueInstEvent
func NewContinueInstEvent() *ContinueInstEvent {
	e := new(ContinueInstEvent)
	e.BasicEvent = core.NewBasicEvent()
	return e
}
