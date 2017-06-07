package cu

import (
	"fmt"
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/timing"
)

// MapWGLog is a LogHook that hooks to a the MapWGReq Event
type MapWGLog struct {
	core.LogHookBase
}

// NewMapWGLog returns a newly created MapWGHook
func NewMapWGLog(logger *log.Logger) *MapWGLog {
	h := new(MapWGLog)
	h.Logger = logger
	return h
}

// Type returns type timing.MapWGReq
func (h *MapWGLog) Type() reflect.Type {
	return reflect.TypeOf((*timing.MapWGReq)(nil))
}

// Pos return AfterEvent
func (h *MapWGLog) Pos() core.HookPos {
	return core.AfterEvent
}

// Func defines the behavior when the hook is triggered
func (h *MapWGLog) Func(item interface{}, domain core.Hookable, info interface{}) {
	req := item.(*timing.MapWGReq)
	wg := req.WG
	str := fmt.Sprintf("%.10f MapWG %d ok: %t, CU: %d\n",
		req.Time(), wg.IDX, req.Ok, req.CUID)
	if req.Ok {
		for _, info := range req.WfDispatchMap {
			str += fmt.Sprintf("\t wf SIMD %d, SGPR offset %d, VGPR offset %d, LDS offset %d\n",
				info.SIMDID, info.SGPROffset, info.VGPROffset, info.LDSOffset)
		}
	}
	h.Logger.Print(str)
}
