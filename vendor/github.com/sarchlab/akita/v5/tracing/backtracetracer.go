package tracing

import (
	"fmt"
	"sync"
)

// TaskPrinter can print tasks with a format.
type TaskPrinter interface {
	Print(task Task)
}

type defaultTaskPrinter struct {
}

func (p *defaultTaskPrinter) Print(task Task) {
	fmt.Printf("%s-%s@%s\n", task.Kind, task.What, task.Location)
}

// BackTraceTracer can record incomplete tasks.
type BackTraceTracer struct {
	NopTracer

	printer      TaskPrinter
	tracingTasks map[uint64]Task
	lock         sync.Mutex
}

// NewBackTraceTracer creates a new BackTraceTracer
func NewBackTraceTracer(printer TaskPrinter) *BackTraceTracer {
	t := &BackTraceTracer{
		printer:      printer,
		tracingTasks: make(map[uint64]Task),
	}

	if t.printer == nil {
		t.printer = &defaultTaskPrinter{}
	}

	return t
}

// StartTask records the started task so it can be back-traced.
func (t *BackTraceTracer) StartTask(task TaskStart) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.tracingTasks[task.ID] = Task{
		ID:        task.ID,
		ParentID:  task.ParentID,
		Kind:      task.Kind,
		What:      task.What,
		Location:  task.Location,
		StartTime: task.Time,
		Detail:    task.Detail,
	}
}

// EndTask removes the completed task.
func (t *BackTraceTracer) EndTask(task TaskEnd) {
	t.lock.Lock()
	defer t.lock.Unlock()

	delete(t.tracingTasks, task.ID)
}

// DumpBackTrace prints the task and the chain of its ancestors.
func (t *BackTraceTracer) DumpBackTrace(task Task) {
	t.printer.Print(task)

	if task.ParentID == 0 {
		return
	}

	parentTask, ok := t.tracingTasks[task.ParentID]
	if !ok {
		return
	}

	t.DumpBackTrace(parentTask)
}
