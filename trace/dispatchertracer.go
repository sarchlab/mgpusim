package trace

import (
	"reflect"
	"sync"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
)

// A DispatcherTracer is a hook that keep record of dispatching status
type DispatcherTracer struct {
	mutex  sync.Mutex
	tracer *Tracer
}

// NewDispatcherTracer creates a new DispatcherTracer.
func NewDispatcherTracer(
	tracer *Tracer,
) *DispatcherTracer {
	t := new(DispatcherTracer)
	t.tracer = tracer
	return t
}

// Type of DispatcherTracer claims that it hooks to any request type
func (t *DispatcherTracer) Type() reflect.Type {
	return reflect.TypeOf((akita.Req)(nil))
}

// Pos of DispatcherTracer returns akita.AnyHookPos. Since DispatcherTracer is not standard hook
// for event or request, it has to use akita.AnyHookPos position.
func (t *DispatcherTracer) Pos() akita.HookPos {
	return akita.AnyHookPos
}

// Func defines the behavior of the tracer when the tracer is invoked.
func (t *DispatcherTracer) Func(
	item interface{},
	domain akita.Hookable,
	info interface{},
) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	req, ok := item.(akita.Req)
	if !ok {
		return
	}

	hInfo, ok := info.(*gcn3.DispatcherHookInfo)
	if !ok {
		return
	}

	switch hInfo.Pos {
	case gcn3.HookPosKernelStart:
		t.tracer.CreateTask(&Task{
			ID:           req.GetID() + ".Kernel",
			ParentTaskID: req.GetID() + ".GPU",
			Type:         "Req",
			What:         reflect.TypeOf(req).String(),
			Where:        domain.(akita.Component).Name(),
			Start:        float64(hInfo.Now),
		})
	case gcn3.HookPosKernelEnd:
		t.tracer.EndTask(req.GetID()+".Kernel", float64(hInfo.Now))
	case gcn3.HookPosWGMapped:
		req := req.(*gcn3.MapWGReq)
		t.tracer.CreateTask(&Task{
			ID:           req.GetID(),
			ParentTaskID: hInfo.KernelDispatchingReq.ID + ".Kernel",
			Type:         "Req",
			What:         "WG_Dispatch",
			Where:        domain.(akita.Component).Name(),
			Start:        float64(hInfo.Now),
		})
	case gcn3.HookPosWGFailed:
		t.tracer.EndTask(req.GetID(), float64(hInfo.Now))
	case gcn3.HookPosWGCompleted:
		t.tracer.EndTask(req.GetID(), float64(hInfo.Now))
	}
}
