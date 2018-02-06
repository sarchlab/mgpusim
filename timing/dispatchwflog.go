package timing

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/util"
)

// DispatchWfLog is the hook that hooks to MapWGEvent
type DispatchWfLog struct {
	util.LogHookBase
}

// NewDispatchWfLog returns a newly created DispatchWfHook
func NewDispatchWfLog(logger *log.Logger) *DispatchWfLog {
	l := new(DispatchWfLog)
	l.Logger = logger
	return l
}

// Type returns type DispatchWfEvent
func (l *DispatchWfLog) Type() reflect.Type {
	return reflect.TypeOf((*DispatchWfEvent)(nil))
}

// Pos return AfterEvent
func (l *DispatchWfLog) Pos() core.HookPos {
	return core.AfterEvent
}

// Func defines the behavior when the hook is triggered
func (l *DispatchWfLog) Func(item interface{}, domain core.Hookable, info interface{}) {
	evt := item.(*DispatchWfEvent)
	l.Logger.Printf("%.10f Dispatch WF: to SIMD %d", evt.Time(),
		evt.Req.Info.SIMDID)
}
