package sample

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/tasks"

	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks/tracing"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cu"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
)

const (
	tableName = "mgpusim_metrics"
)

// secondsOf converts a simulated time in picoseconds to seconds, keeping the
// reported metrics in the same unit as the v4 reports.
func secondsOf(t timing.VTimeInPicoSec) float64 {
	return float64(t) * 1e-12
}

type metric struct {
	Location string
	What     string
	Value    float64
	Unit     string
}

type kernelTimeTracer struct {
	tracer *tracing.BusyTimeTracer
	comp   naming.Named
}

type instCountTracer struct {
	tracer *instTracer
	cu     naming.Named
}

type cacheLatencyTracer struct {
	tracer *tracing.AverageTimeTracer
	cache  naming.Named
}

type cacheHitRateTracer struct {
	tracer *tracing.TagCountTracer
	cache  naming.Named
}

type tlbHitRateTracer struct {
	tracer *tracing.TagCountTracer
	tlb    naming.Named
}

type dramTransactionCountTracer struct {
	tracer *dramTracer
	dram   naming.Named
}

type rdmaTransactionCountTracer struct {
	outgoingTracer *tracing.AverageTimeTracer
	incomingTracer *tracing.AverageTimeTracer
	rdmaEngine     *rdma.Comp
}

type simdBusyTimeTracer struct {
	tracer *tracing.BusyTimeTracer
	simd   naming.Named
}

type cuCPIStackTracer struct {
	cu     naming.Named
	tracer *cu.CPIStackTracer
}

type reporter struct {
	dataRecorder datarecording.DataRecorder
	simulation   *simulation.Simulation

	kernelTimeTracer        *kernelTimeTracer
	perGPUKernelTimeTracers []*kernelTimeTracer
	instCountTracers        []*instCountTracer
	cacheLatencyTracers     []*cacheLatencyTracer
	cacheHitRateTracers     []*cacheHitRateTracer
	tlbHitRateTracers       []*tlbHitRateTracer
	dramTracers             []*dramTransactionCountTracer
	rdmaTransactionCounters []*rdmaTransactionCountTracer
	simdBusyTimeTracers     []*simdBusyTimeTracer
	cuCPITraces             []*cuCPIStackTracer

	ReportInstCount            bool
	ReportCacheLatency         bool
	ReportCacheHitRate         bool
	ReportTLBHitRate           bool
	ReportRDMATransactionCount bool
	ReportDRAMTransactionCount bool
	ReportSIMDBusyTime         bool
	ReportCPIStack             bool
}

func newReporter(s *simulation.Simulation) *reporter {
	r := &reporter{
		dataRecorder: s.DataRecorder(),
		simulation:   s,
	}

	r.injectTracers(s)

	r.dataRecorder.CreateTable(tableName, metric{})

	return r
}

func (r *reporter) injectTracers(s *simulation.Simulation) {
	r.injectKernelTimeTracer(s)
	r.injectInstCountTracer(s)
	r.injectCUCPIHook(s)
	r.injectCacheLatencyTracer(s)
	r.injectCacheHitRateTracer(s)
	r.injectTLBHitRateTracer(s)
	r.injectRDMAEngineTracer(s)
	r.injectDRAMTracer(s)
	r.injectSIMDBusyTimeTracer(s)
}

func (r *reporter) injectKernelTimeTracer(s *simulation.Simulation) {
	var driverComp naming.Named
	for _, c := range s.Components() {
		if c.Name() == "Driver" {
			driverComp = c
			break
		}
	}
	if driverComp == nil {
		panic("runner: missing driver")
	}

	tracer := tracing.NewBusyTimeTracer(
		func(task tasks.Task) bool {
			return task.What == "*driver.LaunchKernelCommand"
		})
	s.TaskTracker().Subscribe(tracer, driverComp)
	r.kernelTimeTracer = &kernelTimeTracer{tracer: tracer, comp: driverComp}

	for _, comp := range s.Components() {
		if strings.Contains(comp.Name(), "CommandProcessor") {
			tracer := tracing.NewBusyTimeTracer(
				func(task tasks.Task) bool {
					return task.What == "LaunchKernelReq"
				})
			s.TaskTracker().Subscribe(tracer, comp.(naming.Named))
			r.perGPUKernelTimeTracers = append(
				r.perGPUKernelTimeTracers,
				&kernelTimeTracer{
					tracer: tracer,
					comp:   comp.(naming.Named),
				})
		}
	}
}

func (r *reporter) injectInstCountTracer(s *simulation.Simulation) {
	if !*reportAll && !*instCountReportFlag {
		return
	}

	for _, comp := range s.Components() {
		if strings.Contains(comp.Name(), "CU") {
			tracer := newInstTracer()
			r.instCountTracers = append(r.instCountTracers,
				&instCountTracer{
					tracer: tracer,
					cu:     comp.(naming.Named),
				})
			s.TaskTracker().Subscribe(tracer, comp.(naming.Named))
		}
	}
}

func (r *reporter) injectCUCPIHook(s *simulation.Simulation) {
	if !*reportAll && !*reportCPIStackFlag {
		return
	}

	for _, comp := range s.Components() {
		if strings.Contains(comp.Name(), "CU") {
			tracer := cu.NewCPIStackInstHook(
				comp.(*cu.Comp), s.Engine())
			s.TaskTracker().Subscribe(tracer, comp.(naming.Named))

			r.cuCPITraces = append(r.cuCPITraces,
				&cuCPIStackTracer{
					tracer: tracer,
					cu:     comp.(naming.Named),
				})
		}
	}
}

func (r *reporter) injectCacheLatencyTracer(s *simulation.Simulation) {
	if !*reportAll && !*cacheLatencyReportFlag {
		return
	}

	for _, comp := range s.Components() {
		if strings.Contains(comp.Name(), "Cache") {
			tracer := tracing.NewAverageTimeTracer(
				func(task tasks.Task) bool {
					return task.Kind == "req_in"
				})
			r.cacheLatencyTracers = append(r.cacheLatencyTracers,
				&cacheLatencyTracer{
					tracer: tracer,
					cache:  comp.(naming.Named),
				})
			s.TaskTracker().Subscribe(tracer, comp.(naming.Named))
		}
	}
}

func (r *reporter) injectCacheHitRateTracer(s *simulation.Simulation) {
	if !*reportAll && !*cacheLatencyReportFlag {
		return
	}

	for _, comp := range s.Components() {
		if strings.Contains(comp.Name(), "Cache") {
			tracer := tracing.NewTagCountTracer(
				func(task tasks.Task) bool { return true })
			r.cacheHitRateTracers = append(r.cacheHitRateTracers,
				&cacheHitRateTracer{
					tracer: tracer,
					cache:  comp.(naming.Named),
				})
			s.TaskTracker().Subscribe(tracer, comp.(naming.Named))
		}
	}
}

func (r *reporter) injectTLBHitRateTracer(s *simulation.Simulation) {
	if !*reportAll && !*tlbHitRateReportFlag {
		return
	}

	for _, comp := range s.Components() {
		if strings.Contains(comp.Name(), "TLB") {
			tracer := tracing.NewTagCountTracer(
				func(task tasks.Task) bool { return true })
			r.tlbHitRateTracers = append(r.tlbHitRateTracers,
				&tlbHitRateTracer{
					tracer: tracer,
					tlb:    comp.(naming.Named),
				})
			s.TaskTracker().Subscribe(tracer, comp.(naming.Named))
		}
	}
}

func (r *reporter) injectRDMAEngineTracer(s *simulation.Simulation) {
	if !*reportAll && !*rdmaTransactionCountReportFlag {
		return
	}

	for _, comp := range s.Components() {
		if strings.Contains(comp.Name(), "RDMA") {
			t := &rdmaTransactionCountTracer{}
			t.rdmaEngine = comp.(*rdma.Comp)
			t.incomingTracer = tracing.NewAverageTimeTracer(
				func(task tasks.Task) bool {
					if task.Kind != "req_in" {
						return false
					}

					msg, ok := task.Detail.(messaging.Msg)
					if !ok {
						return false
					}

					isFromOutside := strings.Contains(
						string(msg.Src), "RDMA")
					if !isFromOutside {
						return false
					}

					return true
				})
			t.outgoingTracer = tracing.NewAverageTimeTracer(
				func(task tasks.Task) bool {
					if task.Kind != "req_in" {
						return false
					}

					msg, ok := task.Detail.(messaging.Msg)
					if !ok {
						return false
					}

					isFromOutside := strings.Contains(
						string(msg.Src), "RDMA")
					if isFromOutside {
						return false
					}

					return true
				})

			s.TaskTracker().Subscribe(t.incomingTracer, t.rdmaEngine)
			s.TaskTracker().Subscribe(t.outgoingTracer, t.rdmaEngine)

			r.rdmaTransactionCounters = append(r.rdmaTransactionCounters, t)
		}
	}
}

func (r *reporter) injectDRAMTracer(s *simulation.Simulation) {
	if !*reportAll && !*dramTransactionCountReportFlag {
		return
	}

	for _, comp := range s.Components() {
		if strings.Contains(comp.Name(), "DRAM") {
			t := &dramTransactionCountTracer{}
			t.dram = comp.(naming.Named)
			t.tracer = newDramTracer()

			s.TaskTracker().Subscribe(t.tracer, t.dram)

			r.dramTracers = append(r.dramTracers, t)
		}
	}
}

// simdTaskRouter routes pipeline completions without registering SIMD views
// as independent simulation entities. Its routing table is fixed at assembly.
type simdTaskRouter struct {
	tasks.NopObserver
	tracers map[string]*tracing.BusyTimeTracer
}

func (r *simdTaskRouter) TaskEnded(task tasks.Task) {
	if task.Kind != "pipeline" {
		return
	}
	index := strings.LastIndexByte(task.What, '.')
	if index < 0 {
		return
	}
	if tracer := r.tracers[task.What[:index]]; tracer != nil {
		tracer.TaskEnded(task)
	}
}

type simdName string

func (n simdName) Name() string { return string(n) }

func (r *reporter) injectSIMDBusyTimeTracer(s *simulation.Simulation) {
	if !*reportAll && !*simdBusyTimeTracerFlag {
		return
	}
	router := &simdTaskRouter{tracers: make(map[string]*tracing.BusyTimeTracer)}
	for _, comp := range s.Components() {
		cuComp, ok := comp.(*cu.Comp)
		if !ok || !strings.Contains(comp.Name(), "CU") {
			continue
		}
		for i := 0; i < cuComp.Spec.SIMDCount; i++ {
			name := fmt.Sprintf("%s.SIMD%d", comp.Name(), i)
			tracer := tracing.NewBusyTimeTracer(func(task tasks.Task) bool { return task.Kind == "pipeline" })
			r.simdBusyTimeTracers = append(r.simdBusyTimeTracers, &simdBusyTimeTracer{tracer: tracer, simd: simdName(name)})
			router.tracers[name] = tracer
		}
	}
	s.TaskTracker().Subscribe(router)
}

func (r *reporter) report() {
	r.reportKernelTime()
	r.reportInstCount()
	r.reportCPIStack()
	r.reportSIMDBusyTime()
	r.reportCacheLatency()
	r.reportCacheHitRate()
	r.reportTLBHitRate()
	r.reportRDMATransactionCount()
	r.reportDRAMTransactionCount()
	r.reportPageMigration()
}

// reportPageMigration reports the page-migration statistics of the driver
// when page migration is on.
func (r *reporter) reportPageMigration() {
	for _, c := range r.simulation.Components() {
		d, ok := c.(*driver.Comp)
		if !ok || d.Spec.PageMigration.Policy != driver.PageMigrationFault {
			continue
		}

		s := d.State.PageMigration.Stats
		for _, m := range []struct {
			what  string
			value float64
			unit  string
		}{
			{"pm_faults", float64(s.Faults), "count"},
			{"pm_dropped_faults", float64(s.DroppedFaults), "count"},
			{"pm_batches", float64(s.Batches), "count"},
			{"pm_migrations", float64(s.Migrations), "count"},
			{"pm_pages_migrated", float64(s.PagesMigrated), "count"},
			{"pm_bytes_migrated", float64(s.BytesMigrated), "byte"},
			{"pm_releases", float64(s.Releases), "count"},
			{"pm_ignored_faults", float64(s.IgnoredFaults), "count"},
			{"pm_deferred_faults", float64(s.DeferredFaults), "count"},
			{"pm_thrashing_events", float64(s.ThrashingEvents), "count"},
			{"pm_pins", float64(s.Pins), "count"},
			{"pm_unpins", float64(s.Unpins), "count"},
			{"pm_migration_time_sum", secondsOf(timing.VTimeInPicoSec(s.MigrationTimeSum)), "second"},
			{"pm_copy_time_sum", secondsOf(timing.VTimeInPicoSec(s.CopyTimeSum)), "second"},
			{"pm_shootdown_time_sum", secondsOf(timing.VTimeInPicoSec(s.ShootdownTimeSum)), "second"},
		} {
			r.dataRecorder.InsertData(tableName, metric{
				Location: d.Name(), What: m.what, Value: m.value, Unit: m.unit,
			})
		}
	}
}

func (r *reporter) reportKernelTime() {
	kernelTime := secondsOf(r.kernelTimeTracer.tracer.BusyTime())
	r.dataRecorder.InsertData(
		tableName,
		metric{
			Location: r.kernelTimeTracer.comp.Name(),
			What:     "kernel_time",
			Value:    kernelTime,
			Unit:     "second",
		},
	)

	for _, t := range r.perGPUKernelTimeTracers {
		kernelTime := secondsOf(t.tracer.BusyTime())
		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: t.comp.Name(),
				What:     "kernel_time",
				Value:    kernelTime,
				Unit:     "second",
			},
		)
	}
}

func (r *reporter) reportInstCount() {
	kernelTime := secondsOf(r.kernelTimeTracer.tracer.BusyTime())
	for _, t := range r.instCountTracers {
		cuFreq := float64(t.cu.(*cu.Comp).Spec.Freq)
		numCycle := kernelTime * cuFreq

		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: t.cu.Name(),
				What:     "cu_inst_count",
				Value:    float64(t.tracer.count),
				Unit:     "count",
			},
		)

		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: t.cu.Name(),
				What:     "cu_CPI",
				Value:    numCycle / float64(t.tracer.count),
				Unit:     "cycles/inst",
			},
		)

		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: t.cu.Name(),
				What:     "simd_inst_count",
				Value:    float64(t.tracer.simdCount),
				Unit:     "count",
			},
		)

		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: t.cu.Name(),
				What:     "simd_CPI",
				Value:    numCycle / float64(t.tracer.simdCount),
				Unit:     "cycles/inst",
			},
		)
	}
}

func (r *reporter) reportCPIStack() {
	for _, t := range r.cuCPITraces {
		cu := t.cu
		hook := t.tracer

		r.reportCPIStackEntries(hook, cu, false)
		r.reportCPIStackEntries(hook, cu, true)
	}
}

func (r *reporter) reportCPIStackEntries(
	hook *cu.CPIStackTracer,
	cu naming.Named,
	simdStack bool,
) {
	cpiStack := hook.GetCPIStack()
	if simdStack {
		cpiStack = hook.GetSIMDCPIStack()
	}

	keys := make([]string, 0, len(cpiStack))
	for k := range cpiStack {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	stackTypeName := "CPIStack"
	if simdStack {
		stackTypeName = "SIMDCPIStack"
	}

	for _, name := range keys {
		value := cpiStack[name]
		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: cu.Name(),
				What:     stackTypeName + "." + name,
				Value:    value,
				Unit:     "cycles/inst",
			},
		)
	}
}

func (r *reporter) reportSIMDBusyTime() {
	for _, t := range r.simdBusyTimeTracers {
		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: t.simd.Name(),
				What:     "busy_time",
				Value:    secondsOf(t.tracer.BusyTime()),
				Unit:     "second",
			},
		)
	}
}

func (r *reporter) reportCacheLatency() {
	for _, tracer := range r.cacheLatencyTracers {
		if tracer.tracer.AverageTime() == 0 {
			continue
		}

		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: tracer.cache.Name(),
				What:     "req_average_latency",
				Value:    secondsOf(tracer.tracer.AverageTime()),
				Unit:     "second",
			},
		)
	}
}

func (r *reporter) reportCacheHitRate() {
	for _, tracer := range r.cacheHitRateTracers {
		readHit := tracer.tracer.TagCount("read-hit")
		readMiss := tracer.tracer.TagCount("read-miss")
		readMSHRHit := tracer.tracer.TagCount("read-mshr-hit")
		writeHit := tracer.tracer.TagCount("write-hit")
		writeMiss := tracer.tracer.TagCount("write-miss")
		writeMSHRHit := tracer.tracer.TagCount("write-mshr-hit")

		totalTransaction := readHit + readMiss + readMSHRHit +
			writeHit + writeMiss + writeMSHRHit

		if totalTransaction == 0 {
			continue
		}

		r.dataRecorder.InsertData(tableName, metric{
			Location: tracer.cache.Name(),
			What:     "read-hit",
			Value:    float64(readHit),
			Unit:     "count",
		})
		r.dataRecorder.InsertData(tableName, metric{
			Location: tracer.cache.Name(),
			What:     "read-miss",
			Value:    float64(readMiss),
			Unit:     "count",
		})
		r.dataRecorder.InsertData(tableName, metric{
			Location: tracer.cache.Name(),
			What:     "read-mshr-hit",
			Value:    float64(readMSHRHit),
			Unit:     "count",
		})
		r.dataRecorder.InsertData(tableName, metric{
			Location: tracer.cache.Name(),
			What:     "write-hit",
			Value:    float64(writeHit),
			Unit:     "count",
		})
		r.dataRecorder.InsertData(tableName, metric{
			Location: tracer.cache.Name(),
			What:     "write-miss",
			Value:    float64(writeMiss),
			Unit:     "count",
		})
		r.dataRecorder.InsertData(tableName, metric{
			Location: tracer.cache.Name(),
			What:     "write-mshr-hit",
			Value:    float64(writeMSHRHit),
			Unit:     "count",
		})
	}
}

func (r *reporter) reportTLBHitRate() {
	for _, tracer := range r.tlbHitRateTracers {
		hit := tracer.tracer.TagCount("hit")
		miss := tracer.tracer.TagCount("miss")
		mshrHit := tracer.tracer.TagCount("mshr-hit")

		totalTransaction := hit + miss + mshrHit

		if totalTransaction == 0 {
			continue
		}

		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: tracer.tlb.Name(),
				What:     "hit",
				Value:    float64(hit),
				Unit:     "count",
			},
		)
		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: tracer.tlb.Name(),
				What:     "miss",
				Value:    float64(miss),
				Unit:     "count",
			},
		)
		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: tracer.tlb.Name(),
				What:     "mshr-hit",
				Value:    float64(mshrHit),
				Unit:     "count",
			},
		)
	}
}

func (r *reporter) reportRDMATransactionCount() {
	for _, t := range r.rdmaTransactionCounters {
		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: t.rdmaEngine.Name(),
				What:     "outgoing_trans_count",
				Value:    float64(t.outgoingTracer.TotalCount()),
				Unit:     "count",
			},
		)
		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: t.rdmaEngine.Name(),
				What:     "incoming_trans_count",
				Value:    float64(t.incomingTracer.TotalCount()),
				Unit:     "count",
			},
		)
	}
}

func (r *reporter) reportDRAMTransactionCount() {
	for _, t := range r.dramTracers {
		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: t.dram.Name(),
				What:     "read_trans_count",
				Value:    float64(t.tracer.readCount),
				Unit:     "count",
			},
		)
		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: t.dram.Name(),
				What:     "write_trans_count",
				Value:    float64(t.tracer.writeCount),
				Unit:     "count",
			},
		)
		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: t.dram.Name(),
				What:     "read_avg_latency",
				Value:    t.tracer.readAvgLatency,
				Unit:     "second",
			},
		)
		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: t.dram.Name(),
				What:     "write_avg_latency",
				Value:    t.tracer.writeAvgLatency,
				Unit:     "second",
			},
		)
		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: t.dram.Name(),
				What:     "read_size",
				Value:    float64(t.tracer.readSize),
				Unit:     "bytes",
			},
		)
		r.dataRecorder.InsertData(
			tableName,
			metric{
				Location: t.dram.Name(),
				What:     "write_size",
				Value:    float64(t.tracer.writeSize),
				Unit:     "bytes",
			},
		)
	}
}
