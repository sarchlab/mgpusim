package gcn3

import (
	"reflect"

	"gitlab.com/akita/akita"
)

// WGCompleteLogger is the logger that writes the information of work-group
// completion
type WGCompleteLogger struct {
	akita.LogHookBase
}

// Type returns type timing.MapWGReq
func (l *WGCompleteLogger) Type() reflect.Type {
	return reflect.TypeOf((*WGFinishMesg)(nil))
}

// Pos return AfterEvent
func (l *WGCompleteLogger) Pos() akita.HookPos {
	return akita.OnRecvReq
}

// Func defines the logging action
func (l *WGCompleteLogger) Func(item interface{}, domain akita.Hookable, info interface{}) {
	req := item.(*WGFinishMesg)
	wg := req.WG
	l.Logger.Printf("%.10f, Work-group(%d, %d, %d) completed\n", req.RecvTime(),
		wg.IDX, wg.IDY, wg.IDZ)
}
