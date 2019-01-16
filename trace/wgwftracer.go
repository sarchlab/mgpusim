package trace

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/timing"
	"reflect"
	"sync"
)

// A WGWfTracer is a hook that keep record of work-group start and end time
type WGWfTracer struct {
	mutex  sync.Mutex
	tracer *Tracer
}

// NewWGTracer creates a new WGWfTracer.
func NewWGTracer(
	tracer *Tracer,
) *WGWfTracer {
	t := new(WGWfTracer)
	t.tracer = tracer
	return t
}

// Type of WGWfTracer claims that it hooks to any request type
func (t *WGWfTracer) Type() reflect.Type {
	return nil
}

// Pos of WGWfTracer returns akita.AnyHookPos. Since WGWfTracer is not standard hook
// for event or request, it has to use akita.AnyHookPos position.
func (t *WGWfTracer) Pos() akita.HookPos {
	return akita.AnyHookPos
}

// Func defines the behavior of the tracer when the tracer is invoked.
func (t *WGWfTracer) Func(
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
	case timing.HookPosWfStart:
		wf := item.(*timing.Wavefront)
		t.tracer.CreateTask(&Task{
			ID:           wf.UID,
			ParentTaskID: wf.WG.UID,
			Type:         "WF",
			What:         "WF",
			Where:        domain.(akita.Component).Name(),
			Start:        float64(hInfo.Now),
		})
	case timing.HookPosWfEnd:
		wf := item.(*timing.Wavefront)
		t.tracer.EndTask(wf.UID, float64(hInfo.Now))
	}
}

