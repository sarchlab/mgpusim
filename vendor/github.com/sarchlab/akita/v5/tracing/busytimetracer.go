package tracing

import (
	"container/list"

	"github.com/sarchlab/akita/v5/timing"
)

type taskTimeStartEnd struct {
	start, end timing.VTimeInPicoSec
	completed  bool
}

// BusyTimeTracer traces the time that a domain is processing a kind of task. If
// the task processing time overlaps, this tracer only considers one instance of
// the overlapped time.
type BusyTimeTracer struct {
	NopTracer

	filter        TaskFilter
	inflightTasks map[uint64]*list.Element
	taskTimes     *list.List
	busyTime      timing.VTimeInPicoSec
}

// NewBusyTimeTracer creates a new BusyTimeTracer
func NewBusyTimeTracer(filter TaskFilter) *BusyTimeTracer {
	t := &BusyTimeTracer{
		filter:        filter,
		inflightTasks: make(map[uint64]*list.Element),
		taskTimes:     list.New(),
	}

	t.taskTimes.Init()

	return t
}

// BusyTime returns the total time has been spent on a certain type of tasks.
func (t *BusyTimeTracer) BusyTime() timing.VTimeInPicoSec {
	return t.busyTime
}

// TerminateAllTasks will mark all the tasks as completed.
func (t *BusyTimeTracer) TerminateAllTasks(now timing.VTimeInPicoSec) {
	for e := t.taskTimes.Front(); e != nil; e = e.Next() {
		task := e.Value.(*taskTimeStartEnd)
		if !task.completed {
			task.completed = true
			task.end = now
		}
	}

	t.collapse(now)
}

func (t *BusyTimeTracer) extendTaskTime(
	base *taskTimeStartEnd,
	t2 *taskTimeStartEnd,
) {
	if t2.start < base.start {
		base.start = t2.start
	}

	if t2.end > base.end {
		base.end = t2.end
	}
}

// StartTask records the task start time
func (t *BusyTimeTracer) StartTask(task TaskStart) {
	if t.filter != nil && !t.filter(task) {
		return
	}

	taskTime := &taskTimeStartEnd{start: task.Time}

	elem := t.taskTimes.PushBack(taskTime)
	t.inflightTasks[task.ID] = elem
}

// EndTask records the end of the task
func (t *BusyTimeTracer) EndTask(task TaskEnd) {
	originalTask, ok := t.inflightTasks[task.ID]
	if !ok {
		return
	}

	time := originalTask.Value.(*taskTimeStartEnd)
	time.end = task.Time
	time.completed = true

	delete(t.inflightTasks, task.ID)

	t.collapse(task.Time)
}

func (t *BusyTimeTracer) collapse(now timing.VTimeInPicoSec) {
	time, found := t.startTimeOfFirstImcompleteTask()
	if found && time < now {
		return
	}

	finishedTasks := make([]*taskTimeStartEnd, 0)

	var next *list.Element
	for e := t.taskTimes.Front(); e != nil; e = next {
		next = e.Next()

		task := e.Value.(*taskTimeStartEnd)
		if !task.completed {
			break
		}

		if task.completed && task.end <= now {
			finishedTasks = append(finishedTasks, task)

			t.taskTimes.Remove(e)
		}
	}

	t.busyTime += t.taskBusyTime(finishedTasks)
}

func (t *BusyTimeTracer) startTimeOfFirstImcompleteTask() (
	timing.VTimeInPicoSec, bool,
) {
	for e := t.taskTimes.Front(); e != nil; e = e.Next() {
		task := e.Value.(*taskTimeStartEnd)
		if !task.completed {
			return task.start, true
		}
	}

	return 0, false
}

func (t *BusyTimeTracer) taskBusyTime(
	tasks []*taskTimeStartEnd,
) timing.VTimeInPicoSec {
	busyTime := timing.VTimeInPicoSec(0)
	coveredMask := make(map[int]bool)

	for i, t1 := range tasks {
		if _, covered := coveredMask[i]; covered {
			continue
		}

		coveredMask[i] = true

		extTime := taskTimeStartEnd{
			start: t1.start,
			end:   t1.end,
		}

		for j, t2 := range tasks {
			if _, covered := coveredMask[j]; covered {
				continue
			}

			if t.taskTimeOverlap(t1, t2) {
				coveredMask[j] = true

				t.extendTaskTime(&extTime, t2)
			}
		}

		busyTime += extTime.end - extTime.start
	}

	return busyTime
}

func (t *BusyTimeTracer) taskTimeOverlap(t1, t2 *taskTimeStartEnd) bool {
	if t1.start <= t2.start && t1.end >= t2.start {
		return true
	}

	if t1.start <= t2.end && t1.end >= t2.end {
		return true
	}

	if t1.start >= t2.start && t1.end <= t2.end {
		return true
	}

	return false
}
