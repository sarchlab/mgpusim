package cu

import (
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/akita/v3/tracing"
)

// InstTracer is a tracer that traces the time that VALU instructions take and VMem instructions take
type InstTracer struct {
	timeTeller    sim.TimeTeller
	startTime     float64
	endTime       float64
	totalVALUTime float64
	totalVMemTime float64
}

// NewInstTracer creates a new InstTracer
func NewInstTracer(timeTeller sim.TimeTeller) *InstTracer {
	return &InstTracer{
		timeTeller:    timeTeller,
		startTime:     0,
		endTime:       0,
		totalVALUTime: 0,
		totalVMemTime: 0,
	}
}

// StartTask begins the tracing for the InstTracer
func (t *InstTracer) StartTask(task tracing.Task) {
	if task.Kind != "inst" {
		return
	}

	if task.What == "VALU" {
		t.endTime = float64(t.timeTeller.CurrentTime())
		timeDiff := t.endTime - t.startTime
		t.totalVALUTime += timeDiff
	} else if task.What == "VMem" {
		t.endTime = float64(t.timeTeller.CurrentTime())
		timeDiff := t.endTime - t.startTime
		t.totalVMemTime += timeDiff
	} else {
		t.endTime = 0
	}

	t.startTime = float64(t.timeTeller.CurrentTime())
}

// StepTask does nothing for now
func (t *InstTracer) StepTask(task tracing.Task) {
	// Do nothing
}

// EndTask does nothing for now
func (t *InstTracer) EndTask(task tracing.Task) {
	// Do nothing
}
