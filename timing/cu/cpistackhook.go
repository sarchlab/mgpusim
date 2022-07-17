package cu

import (
	"fmt"

	"github.com/tebeka/atexit"
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/akita/v3/tracing"
)

// A CPIStackInstHook is a hook to the CU that captures what instructions are
// issued in each cycle.
type CPIStackInstHook struct {
	timeTeller sim.TimeTeller
	cu         *ComputeUnit

	inflightWfs    map[string]tracing.Task
	firstWFStarted bool
	firstWFStart   float64
	lastWFEnd      float64
}

// NewCPIStackInstHook creates a CPIStackInstHook object.
func NewCPIStackInstHook(cu *ComputeUnit, timeTeller sim.TimeTeller) *CPIStackInstHook {
	h := &CPIStackInstHook{
		cu:         cu,
		timeTeller: timeTeller,

		inflightWfs: make(map[string]tracing.Task),
	}

	atexit.Register(func() {
		h.Report()
	})

	return h
}

// Report reports the data collected.
func (h *CPIStackInstHook) Report() {
	totalTime := h.lastWFEnd - h.firstWFStart

	if totalTime == 0 {
		return
	}

	fmt.Printf("%s, total_time, %.10f\n",
		h.cu.Name(), h.lastWFEnd-h.firstWFStart)
}

// Func records issued instructions.
func (h *CPIStackInstHook) Func(ctx sim.HookCtx) {
	switch ctx.Pos {
	case tracing.HookPosTaskStart:
		task := ctx.Item.(tracing.Task)
		h.handleTaskStart(task)
	case tracing.HookPosTaskEnd:
		task := ctx.Item.(tracing.Task)
		h.handleTaskEnd(task)
	default:
		return
	}

	// cu := ctx.Domain.(*ComputeUnit)
	// task := ctx.Item.(tracing.Task)

	// switch task.Kind {

	// }

	// // fmt.Printf("%.10f, %s, %s\n",
	// // 	h.timeTeller.CurrentTime(), cu.Name(), ctx.Pos.Name)

	// fmt.Printf("\tTask %s-%s starts\n", task.Kind, task.What)

}

func (h *CPIStackInstHook) handleTaskStart(task tracing.Task) {
	switch task.Kind {
	case "wavefront":
		h.inflightWfs[task.ID] = task
		if !h.firstWFStarted {
			h.firstWFStarted = true
			h.firstWFStart = float64(h.timeTeller.CurrentTime())
		}
	}
}

func (h *CPIStackInstHook) handleTaskEnd(task tracing.Task) {
	_, ok := h.inflightWfs[task.ID]
	if ok {
		delete(h.inflightWfs, task.ID)
		h.lastWFEnd = float64(h.timeTeller.CurrentTime())
	}
}
