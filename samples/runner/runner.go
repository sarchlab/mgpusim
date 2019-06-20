package runner

import (
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/benchmarks"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/platform"
	"gitlab.com/akita/util/tracing"
)

var timingFlag = flag.Bool("timing", false, "Run detailed timing simulation.")
var parallelFlag = flag.Bool("parallel", false, "Run the simulation in parallel.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var visTracing = flag.Bool("trace-vis", false,
	"Generate trace for visualization purposes.")
var verifyFlag = flag.Bool("verify", false, "Verify the emulation result.")
var memTracing = flag.Bool("trace-mem", false, "Generate memory trace")
var gpuFlag = flag.String("gpus", "1",
	"The GPUs to use, use a format like 1,2,3,4")

// Runner is a class that helps running the benchmarks in the official samples.
type Runner struct {
	Engine                  akita.Engine
	GPUDriver               *driver.Driver
	KernelTimeCounter       *tracing.BusyTimeTracer
	PerGPUKernelTimeCounter []*tracing.BusyTimeTracer
	Benchmarks              []benchmarks.Benchmark
	Timing                  bool
	Verify                  bool
	Parallel                bool

	GPUIDs []int
}

// ParseFlag applies the runner flag to runner object
func (r *Runner) ParseFlag() *Runner {
	if *parallelFlag {
		r.Parallel = true
	}

	if *isaDebug {
		platform.DebugISA = true
	}

	if *visTracing {
		platform.TraceVis = true
	}

	if *memTracing {
		platform.TraceMem = true
	}

	if *verifyFlag {
		r.Verify = true
	}

	if *timingFlag {
		r.Timing = true
	}

	r.parseGPUFlag()
	return r
}

// Init initializes the platform simulate
func (r *Runner) Init() *Runner {

	if r.Parallel {
		platform.UseParallelEngine = true
	}
	if r.Timing {
		r.Engine, r.GPUDriver = platform.BuildNR9NanoPlatform(4)
	} else {
		r.Engine, _, r.GPUDriver, _ = platform.BuildEmuPlatform()
	}

	r.KernelTimeCounter = tracing.NewBusyTimeTracer(
		func(task tracing.Task) bool {
			return task.What == "*driver.LaunchKernelCommand"
		})
	tracing.CollectTrace(r.GPUDriver, r.KernelTimeCounter)

	for _, gpu := range r.GPUDriver.GPUs {
		gpuKernelTimeCountner := tracing.NewBusyTimeTracer(
			func(task tracing.Task) bool {
				return task.What == "Launch Kernel"
			})
		r.PerGPUKernelTimeCounter = append(
			r.PerGPUKernelTimeCounter, gpuKernelTimeCountner)
		tracing.CollectTrace(
			gpu.CommandProcessor.Component().(*gcn3.CommandProcessor),
			gpuKernelTimeCountner)
	}

	return r
}

func (r *Runner) parseGPUFlag() {
	r.GPUIDs = make([]int, 0)
	gpuIDTokens := strings.Split(*gpuFlag, ",")
	for _, t := range gpuIDTokens {
		gpuID, err := strconv.Atoi(t)
		if err != nil {
			log.Fatal(err)
		}
		r.GPUIDs = append(r.GPUIDs, gpuID)
	}
}

// AddBenchmark adds an benchmark that the driver runs
func (r *Runner) AddBenchmark(b benchmarks.Benchmark) {
	b.SelectGPU(r.GPUIDs)
	r.Benchmarks = append(r.Benchmarks, b)
}

// AddBenchmarkWithoutSettingGPUsToUse allows for user specified GPUs for
// the benchmark to run.
func (r *Runner) AddBenchmarkWithoutSettingGPUsToUse(b benchmarks.Benchmark) {
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

	fmt.Printf("Kernel time: %.12f\n", r.KernelTimeCounter.BusyTime())
	fmt.Printf("Total time: %.12f\n", r.Engine.CurrentTime())
	for i, c := range r.PerGPUKernelTimeCounter {
		fmt.Printf("GPU %d kernel time: %.12f\n", i+1, c.BusyTime())
	}
}
