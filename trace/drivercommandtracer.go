package trace

import (
	"reflect"
	"sync"

	"github.com/rs/xid"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/driver"
)

// A DriverCommandTracer is a LogHook that keep record of instruction execution
// status
type DriverCommandTracer struct {
	mutex          sync.Mutex
	tracer         *Tracer
	simulationTask *Task
	cmds           map[driver.Command]*Task
}

// NewDriverCommandTracer creates a new DriverCommandTracer.
func NewDriverCommandTracer(
	tracer *Tracer,
) *DriverCommandTracer {
	t := new(DriverCommandTracer)
	t.tracer = tracer
	t.simulationTask = &Task{
		ID:   xid.New().String(),
		Type: "Simulation",
	}
	t.cmds = make(map[driver.Command]*Task)
	return t
}

// Type of DriverCommandTracer claims the inst tracer is hooking to the timing.Wavefront type
func (t *DriverCommandTracer) Type() reflect.Type {
	return reflect.TypeOf((*driver.Command)(nil))
}

// Pos of DriverCommandTracer returns akita.AnyHookPos. Since DriverCommandTracer is not standard hook
// for event or request, it has to use akita.AnyHookPos position.
func (t *DriverCommandTracer) Pos() akita.HookPos {
	return akita.AnyHookPos
}

// Func defines the behavior of the tracer when the tracer is invoked.
func (t *DriverCommandTracer) Func(
	item interface{},
	domain akita.Hookable,
	info interface{},
) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	cmd := item.(driver.Command)
	cmdInfo := info.(*driver.CommandHookInfo)
	driver := domain.(akita.Component)
	if cmdInfo.IsStart {
		task := &Task{
			ID:           xid.New().String(),
			ParentTaskID: t.simulationTask.ID,
			Type:         "Command",
			What:         reflect.TypeOf(cmd).String(),
			Where:        driver.Name(),
			Start:        float64(cmdInfo.Now),
		}
		t.cmds[cmd] = task

		if task.Start < t.simulationTask.Start {
			t.simulationTask.Start = task.Start
		}
	} else {
		task := t.cmds[cmd]
		task.End = float64(cmdInfo.Now)

		delete(t.cmds, cmd)

		if task.End > t.simulationTask.End {
			t.simulationTask.End = task.End
		}

		t.tracer.Trace(task)
	}
}

func (t *DriverCommandTracer) Finalize() {
	t.tracer.Trace(t.simulationTask)
}
