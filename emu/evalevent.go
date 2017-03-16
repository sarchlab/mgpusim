package emu

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/disasm"
)

// An EvalEvent is the event that happens after the cu fetches the instruction
// from memory. In this event, the instruction is decoded and evaluated.
type EvalEvent struct {
	*core.BasicEvent
	Buf    []byte              // The raw inst buffer
	Inst   *disasm.Instruction // The decoded instruction
	Status interface{}         // For spanning one inst to multiple cycles.
}

// NewEvalEvent create a new EvalEvent
func NewEvalEvent() *EvalEvent {
	e := new(EvalEvent)
	e.BasicEvent = core.NewBasicEvent()
	return e
}
