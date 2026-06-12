package tracing

import (
	"sync"

	"github.com/sarchlab/akita/v5/sim"
)

// AverageTimeTracer can collect the total time of executing a certain type of
// task. If the execution of two tasks overlaps, this tracer will simply add
// the two task processing time together.
type AverageTimeTracer struct {
	timeTeller    sim.TimeTeller
	filter        TaskFilter
	lock          sync.Mutex
	averageTime   sim.VTimeInSec
	inflightTasks map[uint64]Task
	taskCount     uint64
}

// NewAverageTimeTracer creates a new AverageTimeTracer
func NewAverageTimeTracer(
	timeTeller sim.TimeTeller,
	filter TaskFilter,
) *AverageTimeTracer {
	t := &AverageTimeTracer{
		timeTeller:    timeTeller,
		filter:        filter,
		inflightTasks: make(map[uint64]Task),
	}

	return t
}

// AverageTime returns the total time has been spent on a certain type of tasks.
func (t *AverageTimeTracer) AverageTime() sim.VTimeInSec {
	t.lock.Lock()
	time := t.averageTime
	t.lock.Unlock()

	return time
}

// TotalCount returns the total number of tasks.
func (t *AverageTimeTracer) TotalCount() uint64 {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.taskCount
}

// StartTask records the task start time
func (t *AverageTimeTracer) StartTask(task Task) {
	task.StartTime = t.timeTeller.CurrentTime()

	if !t.filter(task) {
		return
	}

	t.lock.Lock()
	t.inflightTasks[task.ID] = task
	t.lock.Unlock()
}

// StepTask does nothing
func (t *AverageTimeTracer) StepTask(_ Task) {
	// Do nothing
}

// AddMilestone does nothing
func (t *AverageTimeTracer) AddMilestone(_ Milestone) {
	// Do nothing
}

// EndTask records the end of the task
func (t *AverageTimeTracer) EndTask(task Task) {
	task.EndTime = t.timeTeller.CurrentTime()

	t.lock.Lock()
	originalTask, ok := t.inflightTasks[task.ID]

	if !ok {
		t.lock.Unlock()
		return
	}

	taskTime := task.EndTime - originalTask.StartTime
	t.averageTime = (t.averageTime*sim.VTimeInSec(t.taskCount) + taskTime) /
		sim.VTimeInSec(t.taskCount+1)

	delete(t.inflightTasks, task.ID)

	t.taskCount++
	t.lock.Unlock()
}
