package trace

import (
	"reflect"
	"sync"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/timing"
)

// A InstTracer is a LogHook that keep record of instruction execution status
type InstTracer struct {
	mutex  sync.Mutex
	tracer *Tracer
	insts  map[string]*Task
}

// NewInstTracer creates a new InstTracer
func NewInstTracer(tracer *Tracer) *InstTracer {
	t := new(InstTracer)
	t.tracer = tracer
	t.insts = make(map[string]*Task)
	return t
}

// Type of InstTracer claims the inst tracer is hooking to the timing.Wavefront type
func (t *InstTracer) Type() reflect.Type {
	return reflect.TypeOf((*timing.Wavefront)(nil))
}

// Pos of InstTracer returns akita.AnyHookPos. Since InstTracer is not standard hook
// for event or request, it has to use akita.AnyHookPos position.
func (t *InstTracer) Pos() akita.HookPos {
	return akita.AnyHookPos
}

// Func defines the behavior of the tracer when the tracer is invoked.
func (t *InstTracer) Func(
	item interface{},
	domain akita.Hookable,
	info interface{},
) {
	// t.mutex.Lock()
	// defer t.mutex.Unlock()

	// instInfo := info.(*timing.InstHookInfo)
	// inst := instInfo.Inst

	// task := t.insts[inst.ID]
	// if task == nil {
	// 	task = &Task{
	// 		ID:     inst.ID,
	// 		Detail: InstDetail{Inst: inst.String(nil)},
	// 	}
	// 	t.insts[inst.ID] = task
	// }

	// // fmt.Fprintf(t.writer, "%s,%.15f,%s,%s,\"%s\"\n",
	// // 	inst.ID, instInfo.Now, "", instInfo.Stage, inst.String(nil))
	// step := &Step{
	// 	Start: float64(instInfo.Now),
	// 	Where: domain.(akita.Component).Name(),
	// 	What:  instInfo.Stage,
	// }
	// task.AddStep(step)

	// if instInfo.Stage == "Completed" {
	// 	t.tracer.Trace(task)
	// 	delete(t.insts, inst.ID)
	// }
}

type InstDetail struct {
	Inst string
}
