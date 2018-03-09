package timing

import (
	"fmt"
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/util"
	"gitlab.com/yaotsu/gcn3"
)

// MapWGLog is a LogHook that hooks to a the MapWGReq Event
type MapWGLog struct {
	util.LogHookBase
}

// NewMapWGLog returns a newly created MapWGHook
func NewMapWGLog(logger *log.Logger) *MapWGLog {
	h := new(MapWGLog)
	h.Logger = logger
	return h
}

// Type returns type gcn3.MapWGReq
func (h *MapWGLog) Type() reflect.Type {
	return reflect.TypeOf((*gcn3.MapWGReq)(nil))
}

// Pos return AfterEvent
func (h *MapWGLog) Pos() core.HookPos {
	return core.AfterEvent
}

// Func defines the behavior when the hook is triggered
func (h *MapWGLog) Func(item interface{}, domain core.Hookable, info interface{}) {
	req := item.(*gcn3.MapWGReq)
	wg := req.WG
	str := fmt.Sprintf("%.10f MapWG %d ok: %t\n",
		req.Time(), wg.IDX, req.Ok)
	h.Logger.Print(str)
}
