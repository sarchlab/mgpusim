package runner

import (
	"github.com/tebeka/atexit"
	"gitlab.com/akita/akita/v3/tracing"
)

// instTracer can trace the number of instruction completed.
type instTracer struct {
	count     uint64
	simdInst  bool
	simdCount uint64
	maxCount  uint64

	inflightInst map[string]tracing.Task
}

// newInstTracer creates a tracer that can count the number of instructions.
func newInstTracer() *instTracer {
	t := &instTracer{
		inflightInst: map[string]tracing.Task{},
	}
	return t
}

// newInstStopper with stop the execution after a given number of instructions
// is retired.
func newInstStopper(maxInst uint64) *instTracer {
	t := &instTracer{
		maxCount:     maxInst,
		inflightInst: map[string]tracing.Task{},
	}
	return t
}

func (t *instTracer) StartTask(task tracing.Task) {
	if task.Kind != "inst" {
		return
	}

	if task.What == "VALU" {
		t.simdInst = true
	} else {
		t.simdInst = false
	}

	t.inflightInst[task.ID] = task
}

func (t *instTracer) StepTask(task tracing.Task) {
	// Do nothing
}

func (t *instTracer) EndTask(task tracing.Task) {
	_, found := t.inflightInst[task.ID]
	if !found {
		return
	}

	if t.simdInst {
		t.simdCount++
	}

	delete(t.inflightInst, task.ID)

	t.count++

	if t.maxCount > 0 && t.count >= t.maxCount {
		atexit.Exit(0)
	}
}
