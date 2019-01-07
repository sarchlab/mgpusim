package trace

import (
	"reflect"
	"sync"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
)

// A GPUTracer is a LogHook that keep record of instruction execution
// status
type GPUTracer struct {
	mutex  sync.Mutex
	tracer *Tracer
}

// NewGPUTracer creates a new GPUTracer.
func NewGPUTracer(
	tracer *Tracer,
) *GPUTracer {
	t := new(GPUTracer)
	t.tracer = tracer
	return t
}

// Type of GPUTracer claims that it hooks to any request type
func (t *GPUTracer) Type() reflect.Type {
	return reflect.TypeOf((akita.Req)(nil))
}

// Pos of GPUTracer returns akita.AnyHookPos. Since GPUTracer is not standard hook
// for event or request, it has to use akita.AnyHookPos position.
func (t *GPUTracer) Pos() akita.HookPos {
	return akita.BeforeEventHookPos
}

// Func defines the behavior of the tracer when the tracer is invoked.
func (t *GPUTracer) Func(
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

	d := domain.(*gcn3.GPU)

	if req.Src() == d.Driver {
		t.tracer.CreateTask(&Task{
			ID:           req.GetID() + "." + d.Name(),
			ParentTaskID: req.GetID(),
			Type:         "Req",
			What:         reflect.TypeOf(req).String(),
			Where:        d.Name(),
			Start:        float64(req.RecvTime()),
		})
	} else {
		t.tracer.EndTask(req.GetID()+"."+d.Name(), float64(req.RecvTime()))
	}

}
