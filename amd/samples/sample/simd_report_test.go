package sample

import (
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/tasks/tracing"
	"testing"
)

func TestSIMDTaskRouterPreservesPerUnitBusyTime(t *testing.T) {
	a, b := tracing.NewBusyTimeTracer(nil), tracing.NewBusyTimeTracer(nil)
	router := simdTaskRouter{tracers: map[string]*tracing.BusyTimeTracer{"GPU.CU.SIMD0": a, "GPU.CU.SIMD1": b}}
	router.TaskEnded(tasks.Task{Kind: "pipeline", What: "GPU.CU.SIMD0.VALU", StartTime: 10, EndTime: 20})
	router.TaskEnded(tasks.Task{Kind: "pipeline", What: "GPU.CU.SIMD1.VALU", StartTime: 15, EndTime: 30})
	router.TaskEnded(tasks.Task{Kind: "pipeline", What: "GPU.CU.decode_vector", StartTime: 0, EndTime: 100})
	router.TaskEnded(tasks.Task{Kind: "inst", What: "GPU.CU.SIMD0.VALU", StartTime: 0, EndTime: 100})
	if a.BusyTime() != 10 || b.BusyTime() != 15 {
		t.Fatalf("incorrect SIMD busy times: %d, %d", a.BusyTime(), b.BusyTime())
	}
}
