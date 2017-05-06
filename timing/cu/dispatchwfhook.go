package cu

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
)

// DispatchWfHook is the hook that hooks to MapWGEvent
type DispatchWfHook struct {
}

// NewDispatchWfHook returns a newly created DispatchWfHook
func NewDispatchWfHook() *DispatchWfHook {
	h := new(DispatchWfHook)
	return h
}

// Type returns type timing.MapWGReq
func (h *DispatchWfHook) Type() reflect.Type {
	return reflect.TypeOf((*DispatchWfEvent)(nil))
}

// Pos return AfterEvent
func (h *DispatchWfHook) Pos() core.HookPos {
	return core.AfterEvent
}

// Func defines the behavior when the hook is triggered
func (h *DispatchWfHook) Func(item interface{}, domain core.Hookable) {
	evt := item.(*DispatchWfEvent)
	log.Printf("Dispatch WF: to SIMD %d", evt.Req.Info.SIMDID)
}
