package cu

import (
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/akita/v3/tracing"
)

type InstTracer struct {
	timeTeller    sim.TimeTeller
	startTime     float64
	endTime       float64
	totalVALUTime float64
	totalVMemTime float64
}

func NewInstTracer(timeTeller sim.TimeTeller) *InstTracer { //should this be called to attach a tracer to each cu? yes; one tracer to 1 cu, or 1 tracer to all cu
	return &InstTracer{
		timeTeller:    timeTeller,
		startTime:     0,
		endTime:       0,
		totalVALUTime: 0,
		totalVMemTime: 0,
	}
}

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

func (t *InstTracer) StepTask(task tracing.Task) {
	// Do nothing
}

func (t *InstTracer) EndTask(task tracing.Task) {

}
