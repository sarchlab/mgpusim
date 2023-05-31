package runner

import (
	"strings"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/timing/cu"
	"github.com/sarchlab/mgpusim/v3/timing/rdma"
	"github.com/tebeka/atexit"
)

type instCountTracer struct {
	tracer *instTracer
	cu     TraceableComponent
}

type cacheLatencyTracer struct {
	tracer *tracing.AverageTimeTracer
	cache  TraceableComponent
}

type cacheHitRateTracer struct {
	tracer *tracing.StepCountTracer
	cache  TraceableComponent
}

type tlbHitRateTracer struct {
	tracer *tracing.StepCountTracer
	tlb    TraceableComponent
}

type dramTransactionCountTracer struct {
	tracer *dramTracer
	dram   TraceableComponent
}

type rdmaTransactionCountTracer struct {
	outgoingTracer *tracing.AverageTimeTracer
	incomingTracer *tracing.AverageTimeTracer
	rdmaEngine     *rdma.Engine
}

type simdBusyTimeTracer struct {
	tracer *tracing.BusyTimeTracer
	simd   TraceableComponent
}

type cuCPIStackTracer struct {
	cu     TraceableComponent
	tracer *cu.CPIStackTracer
}

func (r *Runner) defineMetrics() {
	r.metricsCollector = &collector{}
	r.addMaxInstStopper()
	r.addKernelTimeTracer()
	r.addInstCountTracer()
	r.addCUCPIHook()
	r.addCacheLatencyTracer()
	r.addCacheHitRateTracer()
	r.addTLBHitRateTracer()
	r.addRDMAEngineTracer()
	r.addDRAMTracer()
	r.addSIMDBusyTimeTracer()

	atexit.Register(func() { r.reportStats() })
}

func (r *Runner) addKernelTimeTracer() {
	r.kernelTimeCounter = tracing.NewBusyTimeTracer(
		r.platform.Engine,
		func(task tracing.Task) bool {
			return task.What == "*driver.LaunchKernelCommand"
		})
	tracing.CollectTrace(r.platform.Driver, r.kernelTimeCounter)

	for _, gpu := range r.platform.GPUs {
		gpuKernelTimeCounter := tracing.NewBusyTimeTracer(
			r.platform.Engine,
			func(task tracing.Task) bool {
				return task.What == "*protocol.LaunchKernelReq"
			})
		r.perGPUKernelTimeCounter = append(
			r.perGPUKernelTimeCounter, gpuKernelTimeCounter)
		tracing.CollectTrace(gpu.CommandProcessor, gpuKernelTimeCounter)
	}
}

func (r *Runner) addInstCountTracer() {
	if !r.ReportInstCount {
		return
	}

	for _, gpu := range r.platform.GPUs {
		for _, cu := range gpu.CUs {
			tracer := newInstTracer()
			r.instCountTracers = append(r.instCountTracers,
				instCountTracer{
					tracer: tracer,
					cu:     cu,
				})
			tracing.CollectTrace(cu.(tracing.NamedHookable), tracer)
		}
	}
}

func (r *Runner) addCUCPIHook() {
	if !r.ReportCPIStack {
		return
	}

	for _, gpu := range r.platform.GPUs {
		for _, cuComp := range gpu.CUs {
			tracer := cu.NewCPIStackInstHook(
				cuComp.(*cu.ComputeUnit), r.platform.Engine)
			tracing.CollectTrace(cuComp.(tracing.NamedHookable), tracer)

			r.cuCPITraces = append(r.cuCPITraces,
				cuCPIStackTracer{
					tracer: tracer,
					cu:     cuComp,
				})
		}
	}
}

func (r *Runner) addCacheLatencyTracer() {
	if !r.ReportCacheLatency {
		return
	}

	for _, gpu := range r.platform.GPUs {
		for _, cache := range gpu.L1ICaches {
			tracer := tracing.NewAverageTimeTracer(
				r.platform.Engine,
				func(task tracing.Task) bool {
					return task.Kind == "req_in"
				})
			r.cacheLatencyTracers = append(r.cacheLatencyTracers,
				cacheLatencyTracer{tracer: tracer, cache: cache})
			tracing.CollectTrace(cache, tracer)
		}

		for _, cache := range gpu.L1SCaches {
			tracer := tracing.NewAverageTimeTracer(
				r.platform.Engine,
				func(task tracing.Task) bool {
					return task.Kind == "req_in"
				})
			r.cacheLatencyTracers = append(r.cacheLatencyTracers,
				cacheLatencyTracer{tracer: tracer, cache: cache})
			tracing.CollectTrace(cache, tracer)
		}

		for _, cache := range gpu.L1VCaches {
			tracer := tracing.NewAverageTimeTracer(
				r.platform.Engine,
				func(task tracing.Task) bool {
					return task.Kind == "req_in"
				})
			r.cacheLatencyTracers = append(r.cacheLatencyTracers,
				cacheLatencyTracer{tracer: tracer, cache: cache})
			tracing.CollectTrace(cache, tracer)
		}

		for _, cache := range gpu.L2Caches {
			tracer := tracing.NewAverageTimeTracer(
				r.platform.Engine,
				func(task tracing.Task) bool {
					return task.Kind == "req_in"
				})
			r.cacheLatencyTracers = append(r.cacheLatencyTracers,
				cacheLatencyTracer{tracer: tracer, cache: cache})
			tracing.CollectTrace(cache, tracer)
		}
	}
}

func (r *Runner) addCacheHitRateTracer() {
	if !r.ReportCacheHitRate {
		return
	}

	for _, gpu := range r.platform.GPUs {
		for _, cache := range gpu.L1VCaches {
			tracer := tracing.NewStepCountTracer(
				func(task tracing.Task) bool { return true })
			r.cacheHitRateTracers = append(r.cacheHitRateTracers,
				cacheHitRateTracer{tracer: tracer, cache: cache})
			tracing.CollectTrace(cache, tracer)
		}

		for _, cache := range gpu.L1SCaches {
			tracer := tracing.NewStepCountTracer(
				func(task tracing.Task) bool { return true })
			r.cacheHitRateTracers = append(r.cacheHitRateTracers,
				cacheHitRateTracer{tracer: tracer, cache: cache})
			tracing.CollectTrace(cache, tracer)
		}

		for _, cache := range gpu.L1ICaches {
			tracer := tracing.NewStepCountTracer(
				func(task tracing.Task) bool { return true })
			r.cacheHitRateTracers = append(r.cacheHitRateTracers,
				cacheHitRateTracer{tracer: tracer, cache: cache})
			tracing.CollectTrace(cache, tracer)
		}

		for _, cache := range gpu.L2Caches {
			tracer := tracing.NewStepCountTracer(
				func(task tracing.Task) bool { return true })
			r.cacheHitRateTracers = append(r.cacheHitRateTracers,
				cacheHitRateTracer{tracer: tracer, cache: cache})
			tracing.CollectTrace(cache, tracer)
		}
	}
}

func (r *Runner) addTLBHitRateTracer() {
	if !r.ReportTLBHitRate {
		return
	}

	for _, gpu := range r.platform.GPUs {
		for _, tlb := range gpu.L1VTLBs {
			tracer := tracing.NewStepCountTracer(
				func(task tracing.Task) bool { return true })
			r.tlbHitRateTracers = append(r.tlbHitRateTracers,
				tlbHitRateTracer{tracer: tracer, tlb: tlb})
			tracing.CollectTrace(tlb, tracer)
		}

		for _, tlb := range gpu.L1STLBs {
			tracer := tracing.NewStepCountTracer(
				func(task tracing.Task) bool { return true })
			r.tlbHitRateTracers = append(r.tlbHitRateTracers,
				tlbHitRateTracer{tracer: tracer, tlb: tlb})
			tracing.CollectTrace(tlb, tracer)
		}

		for _, tlb := range gpu.L1ITLBs {
			tracer := tracing.NewStepCountTracer(
				func(task tracing.Task) bool { return true })
			r.tlbHitRateTracers = append(r.tlbHitRateTracers,
				tlbHitRateTracer{tracer: tracer, tlb: tlb})
			tracing.CollectTrace(tlb, tracer)
		}

		for _, tlb := range gpu.L2TLBs {
			tracer := tracing.NewStepCountTracer(
				func(task tracing.Task) bool { return true })
			r.tlbHitRateTracers = append(r.tlbHitRateTracers,
				tlbHitRateTracer{tracer: tracer, tlb: tlb})
			tracing.CollectTrace(tlb, tracer)
		}
	}
}

func (r *Runner) addRDMAEngineTracer() {
	if !r.ReportRDMATransactionCount {
		return
	}

	for _, gpu := range r.platform.GPUs {
		if gpu.RDMAEngine == nil {
			continue
		}

		t := rdmaTransactionCountTracer{}
		t.rdmaEngine = gpu.RDMAEngine
		t.incomingTracer = tracing.NewAverageTimeTracer(
			r.platform.Engine,
			func(task tracing.Task) bool {
				if task.Kind != "req_in" {
					return false
				}

				isFromOutside := strings.Contains(
					task.Detail.(sim.Msg).Meta().Src.Name(), "RDMA")
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
					task.Detail.(sim.Msg).Meta().Src.Name(), "RDMA")
				if isFromOutside {
					return false
				}

				return true
			})

		tracing.CollectTrace(t.rdmaEngine, t.incomingTracer)
		tracing.CollectTrace(t.rdmaEngine, t.outgoingTracer)

		r.rdmaTransactionCounters = append(r.rdmaTransactionCounters, t)
	}
}

func (r *Runner) addDRAMTracer() {
	if !r.ReportDRAMTransactionCount {
		return
	}

	for _, gpu := range r.platform.GPUs {
		for _, dram := range gpu.MemControllers {
			t := dramTransactionCountTracer{}
			t.dram = dram.(TraceableComponent)
			t.tracer = newDramTracer()

			tracing.CollectTrace(t.dram, t.tracer)

			r.dramTracers = append(r.dramTracers, t)
		}
	}
}

func (r *Runner) addSIMDBusyTimeTracer() {
	if !r.ReportSIMDBusyTime {
		return
	}

	for _, gpu := range r.platform.GPUs {
		for _, simd := range gpu.SIMDs {
			perSIMDBusyTimeTracer := tracing.NewBusyTimeTracer(
				r.platform.Engine,
				func(task tracing.Task) bool {
					return task.Kind == "pipeline"
				})
			r.simdBusyTimeTracers = append(r.simdBusyTimeTracers,
				simdBusyTimeTracer{
					tracer: perSIMDBusyTimeTracer,
					simd:   simd,
				})
			tracing.CollectTrace(simd, perSIMDBusyTimeTracer)
		}
	}
}

func (r *Runner) reportStats() {
	r.reportExecutionTime()
	r.reportInstCount()
	r.reportCPIStack()
	r.reportSIMDBusyTime()
	r.reportCacheLatency()
	r.reportCacheHitRate()
	r.reportTLBHitRate()
	r.reportRDMATransactionCount()
	r.reportDRAMTransactionCount()
	r.dumpMetrics()
}

func (r *Runner) reportInstCount() {
	kernelTime := float64(r.kernelTimeCounter.BusyTime())
	for _, t := range r.instCountTracers {
		cuFreq := float64(t.cu.(*cu.ComputeUnit).Freq)
		numCycle := kernelTime * cuFreq

		r.metricsCollector.Collect(
			t.cu.Name(), "cu_inst_count", float64(t.tracer.count))

		r.metricsCollector.Collect(
			t.cu.Name(), "cu_CPI", numCycle/float64(t.tracer.count))

		r.metricsCollector.Collect(
			t.cu.Name(), "simd_inst_count", float64(t.tracer.simdCount))

		r.metricsCollector.Collect(
			t.cu.Name(), "simd_CPI", numCycle/float64(t.tracer.simdCount))
	}
}

func (r *Runner) reportCPIStack() {
	for _, t := range r.cuCPITraces {
		cu := t.cu
		hook := t.tracer

		cpiStack := hook.GetCPIStack()
		for name, value := range cpiStack {
			r.metricsCollector.Collect(cu.Name(), "CPIStack."+name, value)
		}

		simdCPIStack := hook.GetSIMDCPIStack()
		for name, value := range simdCPIStack {
			r.metricsCollector.Collect(cu.Name(), "SIMDCPIStack."+name, value)
		}
	}
}

func (r *Runner) reportSIMDBusyTime() {
	for _, t := range r.simdBusyTimeTracers {
		r.metricsCollector.Collect(
			t.simd.Name(), "busy_time", float64(t.tracer.BusyTime()))
	}
}

func (r *Runner) reportExecutionTime() {
	if r.Timing {
		r.metricsCollector.Collect(
			r.platform.Driver.Name(),
			"kernel_time", float64(r.kernelTimeCounter.BusyTime()))
		r.metricsCollector.Collect(
			r.platform.Driver.Name(),
			"total_time", float64(r.platform.Engine.CurrentTime()))

		for i, c := range r.perGPUKernelTimeCounter {
			r.metricsCollector.Collect(
				r.platform.GPUs[i].CommandProcessor.Name(),
				"kernel_time", float64(c.BusyTime()))
		}
	}
}

func (r *Runner) reportCacheLatency() {
	for _, tracer := range r.cacheLatencyTracers {
		if tracer.tracer.AverageTime() == 0 {
			continue
		}

		r.metricsCollector.Collect(
			tracer.cache.Name(),
			"req_average_latency",
			float64(tracer.tracer.AverageTime()),
		)
	}
}

func (r *Runner) reportCacheHitRate() {
	for _, tracer := range r.cacheHitRateTracers {
		readHit := tracer.tracer.GetStepCount("read-hit")
		readMiss := tracer.tracer.GetStepCount("read-miss")
		readMSHRHit := tracer.tracer.GetStepCount("read-mshr-miss")
		writeHit := tracer.tracer.GetStepCount("write-hit")
		writeMiss := tracer.tracer.GetStepCount("write-miss")
		writeMSHRHit := tracer.tracer.GetStepCount("write-mshr-miss")

		totalTransaction := readHit + readMiss + readMSHRHit +
			writeHit + writeMiss + writeMSHRHit

		if totalTransaction == 0 {
			continue
		}

		r.metricsCollector.Collect(
			tracer.cache.Name(), "read-hit", float64(readHit))
		r.metricsCollector.Collect(
			tracer.cache.Name(), "read-miss", float64(readMiss))
		r.metricsCollector.Collect(
			tracer.cache.Name(), "read-mshr-hit", float64(readMSHRHit))
		r.metricsCollector.Collect(
			tracer.cache.Name(), "write-hit", float64(writeHit))
		r.metricsCollector.Collect(
			tracer.cache.Name(), "write-miss", float64(writeMiss))
		r.metricsCollector.Collect(
			tracer.cache.Name(), "write-mshr-hit", float64(writeMSHRHit))
	}
}

func (r *Runner) reportTLBHitRate() {
	for _, tracer := range r.tlbHitRateTracers {
		hit := tracer.tracer.GetStepCount("hit")
		miss := tracer.tracer.GetStepCount("miss")
		mshrHit := tracer.tracer.GetStepCount("mshr-hit")

		totalTransaction := hit + miss + mshrHit

		if totalTransaction == 0 {
			continue
		}

		r.metricsCollector.Collect(
			tracer.tlb.Name(), "hit", float64(hit))
		r.metricsCollector.Collect(
			tracer.tlb.Name(), "miss", float64(miss))
		r.metricsCollector.Collect(
			tracer.tlb.Name(), "mshr-hit", float64(mshrHit))
	}
}

func (r *Runner) reportRDMATransactionCount() {
	for _, t := range r.rdmaTransactionCounters {
		r.metricsCollector.Collect(
			t.rdmaEngine.Name(),
			"outgoing_trans_count",
			float64(t.outgoingTracer.TotalCount()),
		)
		r.metricsCollector.Collect(
			t.rdmaEngine.Name(),
			"incoming_trans_count",
			float64(t.incomingTracer.TotalCount()),
		)
	}
}

func (r *Runner) reportDRAMTransactionCount() {
	for _, t := range r.dramTracers {
		r.metricsCollector.Collect(
			t.dram.Name(),
			"read_trans_count",
			float64(t.tracer.readCount),
		)
		r.metricsCollector.Collect(
			t.dram.Name(),
			"write_trans_count",
			float64(t.tracer.writeCount),
		)
		r.metricsCollector.Collect(
			t.dram.Name(),
			"read_avg_latency",
			float64(t.tracer.readAvgLatency),
		)
		r.metricsCollector.Collect(
			t.dram.Name(),
			"write_avg_latency",
			float64(t.tracer.writeAvgLatency),
		)
		r.metricsCollector.Collect(
			t.dram.Name(),
			"read_size",
			float64(t.tracer.readSize),
		)
		r.metricsCollector.Collect(
			t.dram.Name(),
			"write_size",
			float64(t.tracer.writeSize),
		)
	}
}

func (r *Runner) dumpMetrics() {
	r.metricsCollector.Dump(*filenameFlag)
}
