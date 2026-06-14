package tracing

import (
	"sync"

	"github.com/sarchlab/akita/v5/timing"
)

// AverageTimeTracer can collect the average time of executing a certain type of
// task.
type AverageTimeTracer struct {
	NopTracer

	filter        TaskFilter
	lock          sync.Mutex
	averageTime   timing.VTimeInPicoSec
	inflightTasks map[uint64]timing.VTimeInPicoSec
	taskCount     uint64
}

// NewAverageTimeTracer creates a new AverageTimeTracer
func NewAverageTimeTracer(filter TaskFilter) *AverageTimeTracer {
	return &AverageTimeTracer{
		filter:        filter,
		inflightTasks: make(map[uint64]timing.VTimeInPicoSec),
	}
}

// AverageTime returns the average time spent on a certain type of tasks.
func (t *AverageTimeTracer) AverageTime() timing.VTimeInPicoSec {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.averageTime
}

// TotalCount returns the total number of tasks.
func (t *AverageTimeTracer) TotalCount() uint64 {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.taskCount
}

// StartTask records the task start time
func (t *AverageTimeTracer) StartTask(task TaskStart) {
	if !t.filter(task) {
		return
	}

	t.lock.Lock()
	t.inflightTasks[task.ID] = task.Time
	t.lock.Unlock()
}

// EndTask records the end of the task
func (t *AverageTimeTracer) EndTask(task TaskEnd) {
	t.lock.Lock()
	defer t.lock.Unlock()

	startTime, ok := t.inflightTasks[task.ID]
	if !ok {
		return
	}

	taskTime := task.Time - startTime
	t.averageTime = (t.averageTime*timing.VTimeInPicoSec(t.taskCount) + taskTime) /
		timing.VTimeInPicoSec(t.taskCount+1)

	delete(t.inflightTasks, task.ID)

	t.taskCount++
}
