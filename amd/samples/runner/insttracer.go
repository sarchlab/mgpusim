package runner

import (
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/tebeka/atexit"
)

// instTracer can trace the number of instruction completed.
type instTracer struct {
	tasks.NopObserver

	count     uint64
	simdCount uint64
	maxCount  uint64

	inflightInst map[uint64]tasks.Task
}

// newInstTracer creates a tracer that can count the number of instructions.
func newInstTracer() *instTracer {
	t := &instTracer{
		inflightInst: map[uint64]tasks.Task{},
	}
	return t
}

// newInstStopper with stop the execution after a given number of instructions
// is retired.
func newInstStopper(maxInst uint64) *instTracer {
	t := &instTracer{
		maxCount:     maxInst,
		inflightInst: map[uint64]tasks.Task{},
	}
	return t
}

func (t *instTracer) TaskStarted(task tasks.Task) {
	if task.Kind != "inst" {
		return
	}

	t.inflightInst[task.ID] = task
}

func (t *instTracer) TaskEnded(task tasks.Task) {
	orgTask, found := t.inflightInst[task.ID]
	if !found {
		return
	}

	if orgTask.What == "VALU" {
		t.simdCount++
	}

	delete(t.inflightInst, task.ID)

	t.count++

	if t.maxCount > 0 && t.count >= t.maxCount {
		atexit.Exit(0)
	}
}
