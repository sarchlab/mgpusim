package cu

import (
	"fmt"

	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/akita/v3/tracing"
)

// InstTracer is a tracer that traces the time that VALU instructions take and VMem instructions take
type InstTracer struct {
	timeTeller    sim.TimeTeller
	totalVALUTime float64
	totalVMemTime float64
	inflightInsts map[string]tracing.Task
}

// NewInstTracer creates a new InstTracer
func NewInstTracer(timeTeller sim.TimeTeller) *InstTracer {
	return &InstTracer{
		timeTeller:    timeTeller,
		totalVALUTime: 0,
		totalVMemTime: 0,
		inflightInsts: make(map[string]tracing.Task),
	}
}

// StartTask begins the tracing for the InstTracer
func (t *InstTracer) StartTask(task tracing.Task) {
	if task.Kind != "inst" {
		return
	}

	task.StartTime = t.timeTeller.CurrentTime()

	t.inflightInsts[task.ID] = task
}

// StepTask does nothing for now
func (t *InstTracer) StepTask(task tracing.Task) {
	// Do nothing
}

// EndTask does nothing for now
func (t *InstTracer) EndTask(task tracing.Task) {
	orgTask, ok := t.inflightInsts[task.ID]

	if !ok {
		return
	}

	orgTask.EndTime = t.timeTeller.CurrentTime()
	timeDiff := orgTask.EndTime - orgTask.StartTime

	if orgTask.What == "VALU" {
		t.totalVALUTime += float64(timeDiff)
	} else if orgTask.What == "VMem" {
		t.totalVMemTime += float64(timeDiff)
	}

	delete(t.inflightInsts, task.ID)

	fmt.Printf("%s, %0.10f, %0.10f, %s\n", task.ID, t.totalVALUTime, t.totalVMemTime, orgTask.Where)
}
