package gcn3

import (
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/util"
)

// WGCompleteLogger is the logger that writes the information of work-group
// completion
type WGCompleteLogger struct {
	util.LogHookBase
}

// Type returns type timing.MapWGReq
func (l *WGCompleteLogger) Type() reflect.Type {
	return reflect.TypeOf((*WGFinishMesg)(nil))
}

// Pos return AfterEvent
func (l *WGCompleteLogger) Pos() core.HookPos {
	return core.OnRecvReq
}

// Func defines the logging action
func (l *WGCompleteLogger) Func(item interface{}, domain core.Hookable, info interface{}) {
	req := item.(*WGFinishMesg)
	wg := req.WG
	l.Logger.Printf("%.10f, Work-group(%d, %d, %d) completed\n", req.RecvTime(),
		wg.IDX, wg.IDY, wg.IDZ)
}
