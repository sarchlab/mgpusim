package tracing

import (
	"sync"

	"github.com/sarchlab/akita/v5/timing"
)

// TotalTimeTracer can collect the total time of executing a certain type of
// task. If the execution of two tasks overlaps, this tracer will simply add
// the two task processing time together.
type TotalTimeTracer struct {
	NopTracer

	filter        TaskFilter
	lock          sync.Mutex
	totalTime     timing.VTimeInPicoSec
	inflightTasks map[uint64]timing.VTimeInPicoSec
}

// NewTotalTimeTracer creates a new TotalTimeTracer
func NewTotalTimeTracer(filter TaskFilter) *TotalTimeTracer {
	return &TotalTimeTracer{
		filter:        filter,
		inflightTasks: make(map[uint64]timing.VTimeInPicoSec),
	}
}

// TotalTime returns the total time has been spent on a certain type of tasks.
func (t *TotalTimeTracer) TotalTime() timing.VTimeInPicoSec {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.totalTime
}

// StartTask records the task start time
func (t *TotalTimeTracer) StartTask(task TaskStart) {
	if !t.filter(task) {
		return
	}

	t.lock.Lock()
	t.inflightTasks[task.ID] = task.Time
	t.lock.Unlock()
}

// EndTask records the end of the task
func (t *TotalTimeTracer) EndTask(task TaskEnd) {
	t.lock.Lock()
	defer t.lock.Unlock()

	startTime, ok := t.inflightTasks[task.ID]
	if !ok {
		return
	}

	t.totalTime += task.Time - startTime
	delete(t.inflightTasks, task.ID)
}
