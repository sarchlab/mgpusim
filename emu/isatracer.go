package emu

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/disasm"
)

// IsaTracer is a hook to the ComputeUnit to dump the information of the
// instruction being executed.
type IsaTracer struct {
	logger       *log.Logger
	disassembler *disasm.Disassembler
}

// NewIsaTracer create a new IsaTracer
func NewIsaTracer(logger *log.Logger,
	disassembler *disasm.Disassembler,
) *IsaTracer {
	return &IsaTracer{logger, disassembler}
}

// Type of the IsaTracer that is the evalEvent, where the instruction will
// get evaluated
func (t *IsaTracer) Type() reflect.Type {
	return reflect.TypeOf((*evalEvent)(nil))
}

// Pos the IsaTracer is AfterEvent
func (t *IsaTracer) Pos() core.HookPos {
	return core.AfterEvent
}

// Func defines that when the hook is called, log the instruction executed.
func (t *IsaTracer) Func(item interface{}, domain core.Hookable) {
	evt := item.(*evalEvent)
	inst, err := t.disassembler.Decode(evt.Buf)
	if err != nil {
		t.logger.Panic(err)
	}
	t.logger.Println(evt.Time(), inst)
}
