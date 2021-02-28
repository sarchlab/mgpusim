package runner

import (
	"sync"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/util/tracing"
)

// dramTracer can trace DRAM activities.
type dramTracer struct {
	sync.Mutex

	inflightTasks map[string]tracing.Task

	readCount       int
	writeCount      int
	readAvgLatency  akita.VTimeInSec
	writeAvgLatency akita.VTimeInSec
	readSize        uint64
	writeSize       uint64
}

func newDramTracer() *dramTracer {
	return &dramTracer{
		inflightTasks: make(map[string]tracing.Task),
	}
}

// StartTask records the task start time
func (t *dramTracer) StartTask(task tracing.Task) {
	t.Lock()
	defer t.Unlock()

	t.inflightTasks[task.ID] = task
}

// StepTask does nothing
func (t *dramTracer) StepTask(task tracing.Task) {
	// Do nothing
}

// EndTask records the end of the task
func (t *dramTracer) EndTask(task tracing.Task) {
	t.Lock()
	defer t.Unlock()

	originalTask, ok := t.inflightTasks[task.ID]
	if !ok {
		return
	}

	taskTime := task.EndTime - originalTask.StartTime

	switch originalTask.What {
	case "*mem.ReadReq":
		t.readAvgLatency = akita.VTimeInSec(
			(float64(t.readAvgLatency)*float64(t.readCount) +
				float64(taskTime)) / float64(t.readCount+1))
		t.readCount++
		t.readSize += originalTask.Detail.(*mem.ReadReq).AccessByteSize
	case "*mem.WriteReq":
		t.writeAvgLatency = akita.VTimeInSec(
			(float64(t.writeAvgLatency)*float64(t.writeCount) +
				float64(taskTime)) / float64(t.writeCount+1))
		t.writeCount++
		t.writeSize += uint64(len(originalTask.Detail.(*mem.WriteReq).Data))
	}

	delete(t.inflightTasks, task.ID)
}
