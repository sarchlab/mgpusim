package driver

import "github.com/sarchlab/akita/v3/tracing"

type instTracer struct {
	count        int64
	inflightInst map[string]*tracing.Task
}

func (t *instTracer) StartTask(task tracing.Task) {
	if task.Kind != "inst" {
		return
	}

	t.inflightInst[task.ID] = &task
}

func (t *instTracer) StepTask(task tracing.Task) {
	// Do nothing
}

func (t *instTracer) EndTask(task tracing.Task) {
	_, found := t.inflightInst[task.ID]
	if !found {
		return
	}

	t.count++
	delete(t.inflightInst, task.ID)
}
