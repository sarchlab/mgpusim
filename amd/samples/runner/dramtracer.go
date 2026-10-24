package runner

import (
	"sync"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
)

// dramTracer can trace DRAM activities.
type dramTracer struct {
	sync.Mutex
	tasks.NopObserver

	inflightTasks map[uint64]tasks.Task

	readCount       int
	writeCount      int
	readAvgLatency  float64 // in seconds
	writeAvgLatency float64 // in seconds
	readSize        uint64
	writeSize       uint64
}

func newDramTracer() *dramTracer {
	return &dramTracer{
		inflightTasks: make(map[uint64]tasks.Task),
	}
}

// StartTask records the task start time
func (t *dramTracer) TaskStarted(task tasks.Task) {
	if task.Kind != "req_in" {
		return
	}

	t.Lock()
	defer t.Unlock()

	t.inflightTasks[task.ID] = task
}

// EndTask records the end of the task
func (t *dramTracer) TaskEnded(task tasks.Task) {
	t.Lock()
	defer t.Unlock()

	originalTask, ok := t.inflightTasks[task.ID]
	if !ok {
		return
	}

	taskTime := float64(task.EndTime-originalTask.StartTime) * 1e-12

	msg, ok := originalTask.Detail.(messaging.Msg)
	if !ok {
		return
	}
	switch req := msg.Payload.(type) {
	case memprotocol.ReadReq:
		t.readAvgLatency = (t.readAvgLatency*float64(t.readCount) +
			taskTime) / float64(t.readCount+1)
		t.readCount++
		t.readSize += req.AccessByteSize
	case memprotocol.WriteReq:
		t.writeAvgLatency = (t.writeAvgLatency*float64(t.writeCount) +
			taskTime) / float64(t.writeCount+1)
		t.writeCount++
		t.writeSize += uint64(len(req.Data))
	}

	delete(t.inflightTasks, task.ID)
}
