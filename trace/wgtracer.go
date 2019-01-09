package trace

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/timing"
	"reflect"
	"sync"
)

// A WGTracer is a hook that keep record of work-group start and end time
type WGTracer struct {
	mutex  sync.Mutex
	tracer *Tracer
}

// NewWGTracer creates a new WGTracer.
func NewWGTracer(
	tracer *Tracer,
) *WGTracer {
	t := new(WGTracer)
	t.tracer = tracer
	return t
}

// Type of WGTracer claims that it hooks to any request type
func (t *WGTracer) Type() reflect.Type {
	return nil
}

// Pos of WGTracer returns akita.AnyHookPos. Since WGTracer is not standard hook
// for event or request, it has to use akita.AnyHookPos position.
func (t *WGTracer) Pos() akita.HookPos {
	return akita.AnyHookPos
}

// Func defines the behavior of the tracer when the tracer is invoked.
func (t *WGTracer) Func(
	item interface{},
	domain akita.Hookable,
	info interface{},
) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	hInfo, ok := info.(*timing.CUHookInfo)
	if !ok {
		return
	}

	switch hInfo.Pos {
	case timing.HookPosWGStart:
		req := item.(*gcn3.MapWGReq)
		t.tracer.CreateTask(&Task{
			ID:           req.WG.UID,
			ParentTaskID: req.GetID(),
			Type:         "WG",
			What:         "WG",
			Where:        domain.(akita.Component).Name(),
			Start:        float64(hInfo.Now),
		})
	case timing.HookPosWGEnd:
		wg := item.(*timing.WorkGroup)
		t.tracer.EndTask(wg.UID, float64(hInfo.Now))
	}
}

