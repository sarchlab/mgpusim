package runner

import (
	"strings"

	"github.com/sarchlab/akita/v3/mem/vm/gmmu"
	"github.com/sarchlab/akita/v3/mem/vm/mmu"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
)

type rdmaLatencyTracer struct {
	tracer *tracing.AverageTimeTracer
	rdma   TraceableComponent
}

type mmuLatencyTracer struct {
	tracer *tracing.AverageTimeTracer
	mmu    TraceableComponent
}

type gmmuLatencyTracer struct {
	tracer *tracing.AverageTimeTracer
	gmmu   TraceableComponent
}

type tlbLatencyTracer struct {
	tracer *tracing.AverageTimeTracer
	tlb    TraceableComponent
}

type gmmuTransactionCountTracer struct {
	outgoingTracer *tracing.AverageTimeTracer
	incomingTracer *tracing.AverageTimeTracer
	gmmuEngine     *gmmu.GMMU
}

//	type gmmuTransactionCountTracer struct {
//		tracer *mmuTracer
//		gmmu   *gmmu.GMMU
//	}
type mmuTransactionCountTracer struct {
	outgoingTracer *tracing.AverageTimeTracer
	incomingTracer *tracing.AverageTimeTracer
	mmuEngine      *mmu.MMU
}

type gmmuCacheHitRateTracer struct {
	tracer    *tracing.StepCountTracer
	gmmuCache TraceableComponent
}

type gmmuCacheLatencyTracer struct {
	tracer    *tracing.AverageTimeTracer
	gmmuCache TraceableComponent
}

// func (r *Runner) addSIMDBusyTimeTracer() {
// 	if !r.ReportSIMDBusyTime {
// 		return
// 	}

// 	for _, gpu := range r.platform.GPUs {
// 		for _, simd := range gpu.SIMDs {
// 			perSIMDBusyTimeTracer := tracing.NewBusyTimeTracer(
// 				r.platform.Engine,
// 				func(task tracing.Task) bool {
// 					return task.Kind == "pipeline"
// 				})
// 			r.simdBusyTimeTracers = append(r.simdBusyTimeTracers,
// 				simdBusyTimeTracer{
// 					tracer: perSIMDBusyTimeTracer,
// 					simd:   simd,
// 				})
// 			tracing.CollectTrace(simd, perSIMDBusyTimeTracer)
// 		}
// 	}
// }

func (r *Runner) addMMUEngineTracer() {
	if !r.ReportMMUTransactionCount {
		return
	}

	for _, gpu := range r.platform.GPUs {
		t := mmuTransactionCountTracer{}
		// t.mmuEngine = gpu.MMUEngine
		t.mmuEngine = gpu.MMUEngine
		t.incomingTracer = tracing.NewAverageTimeTracer(
			r.platform.Engine,
			func(task tracing.Task) bool {
				if task.Kind != "req_in" {
					return false
				}

				isFromOutside := strings.Contains(
					task.Detail.(sim.Msg).Meta().Dst.Name(), "MMU")
				if !isFromOutside {
					return false
				}

				return true
			})
		t.outgoingTracer = tracing.NewAverageTimeTracer(
			r.platform.Engine,
			func(task tracing.Task) bool {
				if task.Kind != "req_in" {
					return false
				}

				isFromOutside := strings.Contains(
					task.Detail.(sim.Msg).Meta().Src.Name(), "MMU")
				if isFromOutside {
					return false
				}

				return true
			})

		tracing.CollectTrace(t.mmuEngine, t.incomingTracer)
		tracing.CollectTrace(t.mmuEngine, t.outgoingTracer)

		r.mmuTransactionCounters = append(r.mmuTransactionCounters, t)
	}
}

func (r *Runner) addGMMUEngineTracer() {
	if !r.ReportMMUTransactionCount {
		return
	}

	for _, gpu := range r.platform.GPUs {
		t := gmmuTransactionCountTracer{}
		// t := mmuTransactionCountTracer{}
		t.gmmuEngine = gpu.GMMUEngine
		t.incomingTracer = tracing.NewAverageTimeTracer(
			r.platform.Engine,
			func(task tracing.Task) bool {
				if task.Kind != "req_in" {
					return false
				}

				isFromOutside := strings.Contains(
					task.Detail.(sim.Msg).Meta().Dst.Name(), "GMMU")
				if !isFromOutside {
					return false
				}

				return true
			})
		t.outgoingTracer = tracing.NewAverageTimeTracer(
			r.platform.Engine,
			func(task tracing.Task) bool {
				if task.Kind != "req_in" {
					return false
				}

				isFromOutside := strings.Contains(
					task.Detail.(sim.Msg).Meta().Src.Name(), "GMMU")
				if isFromOutside {
					return false
				}

				return true
			})

		tracing.CollectTrace(t.gmmuEngine, t.incomingTracer)
		tracing.CollectTrace(t.gmmuEngine, t.outgoingTracer)

		r.gmmuTransactionCounters = append(r.gmmuTransactionCounters, t)
	}
}

func (r *Runner) addMMULatencyTracer() {
	if !r.ReportMMULatency {
		return
	}

	for _, gpu := range r.platform.GPUs {
		mmu := gpu.MMUEngine
		tracer := tracing.NewAverageTimeTracer(
			r.platform.Engine,
			func(task tracing.Task) bool {
				return task.Kind == "req_in"
			})
		r.mmuLatencyTracers = append(r.mmuLatencyTracers,
			mmuLatencyTracer{tracer: tracer, mmu: mmu})
		tracing.CollectTrace(mmu, tracer)

	}
}

func (r *Runner) addGMMULatencyTracer() {
	if !r.ReportGMMULatency {
		return
	}

	for _, gpu := range r.platform.GPUs {
		gmmu := gpu.GMMUEngine
		tracer := tracing.NewAverageTimeTracer(
			r.platform.Engine,
			func(task tracing.Task) bool {
				return task.Kind == "req_in"
			})
		r.gmmuLatencyTracers = append(r.gmmuLatencyTracers,
			gmmuLatencyTracer{tracer: tracer, gmmu: gmmu})
		tracing.CollectTrace(gmmu, tracer)

	}
}

func (r *Runner) addGMMUCacheLatencyTracer() {
	if !r.ReportGMMUCacheLatency {
		return
	}

	for _, gpu := range r.platform.GPUs {
		for _, gmmuCache := range gpu.GMMUCache {
			tracer := tracing.NewAverageTimeTracer(
				r.platform.Engine,
				func(task tracing.Task) bool {
					return task.Kind == "req_in"
				})
			r.gmmuCacheLatencyTracers = append(r.gmmuCacheLatencyTracers,
				gmmuCacheLatencyTracer{tracer: tracer, gmmuCache: gmmuCache})
			tracing.CollectTrace(gmmuCache, tracer)
		}
	}
}

func (r *Runner) addGMMUCacheHitRateTracer() {
	if !r.ReportGMMUCacheHitRate {
		return
	}

	for _, gpu := range r.platform.GPUs {
		for _, gmmuCache := range gpu.GMMUCache {
			tracer := tracing.NewStepCountTracer(
				func(task tracing.Task) bool { return true })
			r.gmmuCacheHitRateTracers = append(r.gmmuCacheHitRateTracers,
				gmmuCacheHitRateTracer{tracer: tracer, gmmuCache: gmmuCache})
			tracing.CollectTrace(gmmuCache, tracer)
		}
	}
}
