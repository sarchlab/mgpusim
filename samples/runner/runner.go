// Package runner defines how default benchmark samples are executed.
package runner

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"

	// Enable profiling
	_ "net/http/pprof"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/tebeka/atexit"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/benchmarks"
	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/platform"
	"gitlab.com/akita/mgpusim/timing/caches/l1v"
	"gitlab.com/akita/util/tracing"
)

var timingFlag = flag.Bool("timing", false, "Run detailed timing simulation.")
var parallelFlag = flag.Bool("parallel", false,
	"Run the simulation in parallel.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var visTracing = flag.Bool("trace-vis", false,
	"Generate trace for visualization purposes.")
var verifyFlag = flag.Bool("verify", false, "Verify the emulation result.")
var memTracing = flag.Bool("trace-mem", false, "Generate memory trace")
var disableProgressBar = flag.Bool("no-progress-bar", false,
	"Disables the progress bar")
var cacheLatencyReportFlag = flag.Bool("report-cache-latency", false,
	"Report the average cache latency.")
var cacheHitRateReportFlag = flag.Bool("report-cache-hit-rate", false,
	"Report the cache hit rate of each cache.")
var gpuFlag = flag.String("gpus", "",
	"The GPUs to use, use a format like 1,2,3,4. By default, GPU 1 is used.")
var unifiedGPUFlag = flag.String("unified-gpus", "",
	`Run multi-GPU benchmark in a unified mode.
Use a format like 1,2,3,4. Cannot coexist with -gpus.`)
var useUnifiedMemoryFlag = flag.Bool("use-unified-memory", false,
	"Run benchmark with Unified Memory or not")

type cacheLatencyTracer struct {
	tracer *tracing.AverageTimeTracer
	cache  akita.Component
}

type cacheHitRateTracer struct {
	tracer *tracing.StepCountTracer
	cache  akita.Component
}

// Runner is a class that helps running the benchmarks in the official samples.
type Runner struct {
	Engine                  akita.Engine
	GPUDriver               *driver.Driver
	KernelTimeCounter       *tracing.BusyTimeTracer
	PerGPUKernelTimeCounter []*tracing.BusyTimeTracer
	CacheLatencyTracers     []cacheLatencyTracer
	CacheHitRateTracers     []cacheHitRateTracer
	Benchmarks              []benchmarks.Benchmark
	Timing                  bool
	Verify                  bool
	Parallel                bool
	ReportCacheLatency      bool
	ReportCacheHitRate      bool
	UseUnifiedMemory        bool

	GPUIDs []int
}

// ParseFlag applies the runner flag to runner object
func (r *Runner) ParseFlag() *Runner {
	if *parallelFlag {
		r.Parallel = true
	}

	if *verifyFlag {
		r.Verify = true
	}

	if *timingFlag {
		r.Timing = true
	}

	if *useUnifiedMemoryFlag {
		r.UseUnifiedMemory = true
	}

	if *cacheLatencyReportFlag {
		r.ReportCacheLatency = true
	}

	if *cacheHitRateReportFlag {
		r.ReportCacheHitRate = true
	}

	return r
}

func (r *Runner) startProfilingServer() {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}

	fmt.Println("Profiling server running on:",
		listener.Addr().(*net.TCPAddr).Port)

	panic(http.Serve(listener, nil))
}

// Init initializes the platform simulate
func (r *Runner) Init() *Runner {
	go r.startProfilingServer()

	r.ParseFlag()

	log.SetFlags(log.Llongfile)

	if r.Timing {
		r.buildTimingPlatform()
	} else {
		r.buildEmuPlatform()
	}

	r.parseGPUFlag()

	r.addKernelTimeTracer()
	r.addCacheLatencyTracer()
	r.addCacheHitRateTracer()

	return r
}

func (r *Runner) buildEmuPlatform() {
	b := platform.MakeEmuBuilder()

	if r.Parallel {
		b = b.WithParallelEngine()
	}

	if *isaDebug {
		b = b.WithISADebugging()
	}

	if *visTracing {
		b = b.WithVisTracing()
	}

	if *memTracing {
		b = b.WithMemTracing()
	}

	if *disableProgressBar {
		b = b.WithoutProgressBar()
	}

	r.Engine, r.GPUDriver = b.Build()
}

func (r *Runner) buildTimingPlatform() {
	b := platform.MakeR9NanoBuilder()

	if r.Parallel {
		b = b.WithParallelEngine()
	}

	if *isaDebug {
		b = b.WithISADebugging()
	}

	if *visTracing {
		b = b.WithVisTracing()
	}

	if *memTracing {
		b = b.WithMemTracing()
	}

	if *disableProgressBar {
		b = b.WithoutProgressBar()
	}

	r.Engine, r.GPUDriver = b.Build()
}

func (r *Runner) addKernelTimeTracer() {
	r.KernelTimeCounter = tracing.NewBusyTimeTracer(
		func(task tracing.Task) bool {
			return task.What == "*driver.LaunchKernelCommand"
		})
	tracing.CollectTrace(r.GPUDriver, r.KernelTimeCounter)

	for _, gpu := range r.GPUDriver.GPUs {
		gpuKernelTimeCountner := tracing.NewBusyTimeTracer(
			func(task tracing.Task) bool {
				return task.What == "*gcn3.LaunchKernelReq"
			})
		r.PerGPUKernelTimeCounter = append(
			r.PerGPUKernelTimeCounter, gpuKernelTimeCountner)
		tracing.CollectTrace(gpu.CommandProcessor, gpuKernelTimeCountner)
	}
}

func (r *Runner) addCacheLatencyTracer() {
	if !r.ReportCacheLatency {
		return
	}

	for _, gpu := range r.GPUDriver.GPUs {
		for _, cache := range gpu.L2Caches {
			tracer := tracing.NewAverageTimeTracer(
				func(task tracing.Task) bool {
					return task.Kind == "req_in"
				})
			r.CacheLatencyTracers = append(r.CacheLatencyTracers,
				cacheLatencyTracer{tracer: tracer, cache: cache})
			tracing.CollectTrace(cache, tracer)
		}
	}
}

func (r *Runner) addCacheHitRateTracer() {
	if !r.ReportCacheHitRate {
		return
	}

	for _, gpu := range r.GPUDriver.GPUs {
		for _, cache := range gpu.L1VCaches {
			tracer := tracing.NewStepCountTracer(
				func(task tracing.Task) bool { return true })
			r.CacheHitRateTracers = append(r.CacheHitRateTracers,
				cacheHitRateTracer{tracer: tracer, cache: cache})
			tracing.CollectTrace(cache.(*l1v.Cache), tracer)
		}

		for _, cache := range gpu.L1SCaches {
			tracer := tracing.NewStepCountTracer(
				func(task tracing.Task) bool { return true })
			r.CacheHitRateTracers = append(r.CacheHitRateTracers,
				cacheHitRateTracer{tracer: tracer, cache: cache})
			tracing.CollectTrace(cache.(*l1v.Cache), tracer)
		}

		for _, cache := range gpu.L1ICaches {
			tracer := tracing.NewStepCountTracer(
				func(task tracing.Task) bool { return true })
			r.CacheHitRateTracers = append(r.CacheHitRateTracers,
				cacheHitRateTracer{tracer: tracer, cache: cache})
			tracing.CollectTrace(cache.(*l1v.Cache), tracer)
		}

		for _, cache := range gpu.L2Caches {
			tracer := tracing.NewStepCountTracer(
				func(task tracing.Task) bool { return true })
			r.CacheHitRateTracers = append(r.CacheHitRateTracers,
				cacheHitRateTracer{tracer: tracer, cache: cache})
			tracing.CollectTrace(cache, tracer)
		}
	}
}

func (r *Runner) parseGPUFlag() {
	if *gpuFlag == "" && *unifiedGPUFlag == "" {
		r.GPUIDs = []int{1}
		return
	}

	if *gpuFlag != "" && *unifiedGPUFlag != "" {
		panic("cannot use -gpus and -unified-gpus together")
	}

	if *unifiedGPUFlag != "" {
		gpuIDs := r.gpuIDStringToList(*unifiedGPUFlag)
		unifiedGPUID := r.GPUDriver.CreateUnifiedGPU(nil, gpuIDs)
		r.GPUIDs = []int{unifiedGPUID}
		return
	}

	gpuIDs := r.gpuIDStringToList(*gpuFlag)
	r.GPUIDs = gpuIDs
}

func (r *Runner) gpuIDStringToList(gpuIDsString string) []int {
	gpuIDs := make([]int, 0)
	gpuIDTokens := strings.Split(gpuIDsString, ",")
	for _, t := range gpuIDTokens {
		gpuID, err := strconv.Atoi(t)
		if err != nil {
			panic(err)
		}
		gpuIDs = append(gpuIDs, gpuID)
	}
	return gpuIDs
}

// AddBenchmark adds an benchmark that the driver runs
func (r *Runner) AddBenchmark(b benchmarks.Benchmark) {
	b.SelectGPU(r.GPUIDs)
	if r.UseUnifiedMemory {
		b.SetUnifiedMemory()
	}
	r.Benchmarks = append(r.Benchmarks, b)
}

// AddBenchmarkWithoutSettingGPUsToUse allows for user specified GPUs for
// the benchmark to run.
func (r *Runner) AddBenchmarkWithoutSettingGPUsToUse(b benchmarks.Benchmark) {
	if r.UseUnifiedMemory {
		b.SetUnifiedMemory()
	}
	r.Benchmarks = append(r.Benchmarks, b)
}

// Run runs the benchmark on the simulator
func (r *Runner) Run() {
	r.GPUDriver.Run()

	var wg sync.WaitGroup
	for _, b := range r.Benchmarks {
		wg.Add(1)
		go func(b benchmarks.Benchmark, wg *sync.WaitGroup) {
			b.Run()
			if r.Verify {
				b.Verify()
			}
			wg.Done()
		}(b, &wg)
	}
	wg.Wait()

	r.GPUDriver.Terminate()
	r.Engine.Finished()

	r.reportStats()

	atexit.Exit(0)
}

func (r *Runner) reportStats() {
	r.reportExecutionTime()
	r.reportCacheLatency()
	r.reportCacheHitRate()
}

func (r *Runner) reportExecutionTime() {
	fmt.Printf("Kernel time: %.12f\n", r.KernelTimeCounter.BusyTime())
	fmt.Printf("Total time: %.12f\n", r.Engine.CurrentTime())
	for i, c := range r.PerGPUKernelTimeCounter {
		fmt.Printf("GPU %d kernel time: %.12f\n", i+1, c.BusyTime())
	}
}

func (r *Runner) reportCacheLatency() {
	for _, tracer := range r.CacheLatencyTracers {
		fmt.Printf("Cache %s average latency %.12f\n",
			tracer.cache.Name(),
			tracer.tracer.AverageTime(),
		)
	}
}

func (r *Runner) reportCacheHitRate() {
	f, err := os.Create("cache_hit.csv")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	fmt.Fprintf(f, "name, read-hit, read-miss, read-mshr-miss, write-hit, write-miss, write-mshr-hit\n")

	for _, tracer := range r.CacheHitRateTracers {
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

		fmt.Fprintf(f, "%s, %d, %d, %d, %d, %d, %d\n",
			tracer.cache.Name(),
			readHit, readMiss, readMSHRHit,
			writeHit, writeMiss, writeMSHRHit)
	}
}
