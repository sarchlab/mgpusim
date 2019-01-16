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
	instTasks  map[string]*Task
	instStageTasks map[string]*Task
}

// NewInstTracer creates a new InstTracer
func NewInstTracer(tracer *Tracer) *InstTracer {
	t := new(InstTracer)
	t.tracer = tracer
	t.instTasks = make(map[string]*Task)
	t.instStageTasks = make(map[string]*Task)
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
	t.mutex.Lock()
	defer t.mutex.Unlock()

	wf := item.(*timing.Wavefront)
	instInfo, ok := info.(*timing.InstHookInfo)
	if !ok {
		return
	}
	inst := instInfo.Inst

	instTask, found := t.instTasks[inst.ID]
	if !found {
		instTask = &Task{
			ID: inst.ID,
			ParentTaskID: wf.UID,
			Type: "Inst",
			What: inst.String(nil),
			Where: domain.(akita.Component).Name(),
			Start: float64(instInfo.Now),
		}
		t.instTasks[inst.ID] = instTask
		t.tracer.CreateTask(instTask)
	}

	stageTask, found := t.instStageTasks[inst.ID]
	if !found{
		stageTask = &Task{
			ID: inst.ID + "." + instInfo.Stage,
			ParentTaskID: inst.ID,
			Type: "Inst Stage",
			What: instInfo.Stage,
			Where: domain.(akita.Component).Name(),
			Start: float64(instInfo.Now),
		}
		t.instStageTasks[inst.ID] = stageTask
		t.tracer.CreateTask(stageTask)
	} else {
		if instInfo.Stage != "Completed" {
			t.tracer.EndTask(stageTask.ID, float64(instInfo.Now))
			stageTask = &Task{
				ID: inst.ID + "." + instInfo.Stage,
				ParentTaskID: inst.ID,
				Type: "Inst Stage",
				What: instInfo.Stage,
				Where: domain.(akita.Component).Name(),
				Start: float64(instInfo.Now),
			}
			t.instStageTasks[inst.ID] = instTask
			t.tracer.CreateTask(stageTask)
		}
	}

	if instInfo.Stage == "Completed" {
		t.tracer.EndTask(stageTask.ID, float64(instInfo.Now))
		t.tracer.EndTask(instTask.ID, float64(instInfo.Now))
		delete(t.instStageTasks, inst.ID)
		delete(t.instStageTasks, inst.ID)
	}
}

type InstDetail struct {
	Inst string
}
