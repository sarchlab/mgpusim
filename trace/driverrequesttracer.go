package trace

import (
	"reflect"
	"sync"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/driver"
)

// A DriverRequestTracer is a LogHook that keep record of instruction execution
// status
type DriverRequestTracer struct {
	mutex  sync.Mutex
	tracer *Tracer
}

// NewDriverRequestTracer creates a new DriverRequestTracer.
func NewDriverRequestTracer(
	tracer *Tracer,
) *DriverRequestTracer {
	t := new(DriverRequestTracer)
	t.tracer = tracer
	return t
}

// Type of DriverRequestTracer claims that it hooks to any request type
func (t *DriverRequestTracer) Type() reflect.Type {
	return reflect.TypeOf((akita.Req)(nil))
}

// Pos of DriverRequestTracer returns akita.AnyHookPos. Since DriverRequestTracer is not standard hook
// for event or request, it has to use akita.AnyHookPos position.
func (t *DriverRequestTracer) Pos() akita.HookPos {
	return akita.AnyHookPos
}

// Func defines the behavior of the tracer when the tracer is invoked.
func (t *DriverRequestTracer) Func(
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

	d := domain.(*driver.Driver)
	hookInfo := info.(*driver.ReqHookInfo)

	if hookInfo.EventType == "CREATE" {
		t.tracer.CreateTask(&Task{
			ID:           req.GetID(),
			ParentTaskID: hookInfo.CommandID,
			Type:         "Req",
			What:         reflect.TypeOf(req).String(),
			Where:        d.Name(),
			Start:        float64(hookInfo.Now),
		})
	} else if hookInfo.EventType == "RETRIEVE" {
		t.tracer.EndTask(req.GetID(), float64(hookInfo.Now))
	}

}
