package cu

import (
	"fmt"
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/timing"
)

// MapWGHook is the hook that hooks to MapWGEvent
type MapWGHook struct {
}

// NewMapWGHook returns a newly created MapWGHook
func NewMapWGHook() *MapWGHook {
	h := new(MapWGHook)
	return h
}

// Type returns type timing.MapWGReq
func (h *MapWGHook) Type() reflect.Type {
	return reflect.TypeOf((*timing.MapWGReq)(nil))
}

// Pos return AfterEvent
func (h *MapWGHook) Pos() core.HookPos {
	return core.AfterEvent
}

// Func defines the behavior when the hook is triggered
func (h *MapWGHook) Func(item interface{}, domain core.Hookable) {
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
	log.Print(str)
}
