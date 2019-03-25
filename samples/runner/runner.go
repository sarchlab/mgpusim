package runner

import (
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/benchmarks"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/platform"
)

var timing = flag.Bool("timing", false, "Run detailed timing simulation.")
var parallel = flag.Bool("parallel", false, "Run the simulation in parallel.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var visTracing = flag.Bool("trace-vis", false,
	"Generate trace for visualization purposes.")
var verify = flag.Bool("verify", false, "Verify the emulation result.")
var memTracing = flag.Bool("trace-mem", false, "Generate memory trace")
var gpuFlag = flag.String("gpus", "1",
	"The GPUs to use, use a format like 1,2,3,4")

// Runner is a class that helps running the benchmarks in the official samples.
type Runner struct {
	Engine            akita.Engine
	GPUDriver         *driver.Driver
	KernelTimeCounter *driver.KernelTimeCounter
	Benchmarks        []benchmarks.Benchmark
	gpuIDs            []int
}

// Init initializes the platform simulate
func (r *Runner) Init() {
	if *parallel {
		platform.UseParallelEngine = true
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

	r.parseGPUFlag()

	r.KernelTimeCounter = driver.NewKernelTimeCounter()
	if *timing {
		r.Engine, r.GPUDriver = platform.BuildNR9NanoPlatform(4)
	} else {
		r.Engine, _, r.GPUDriver, _ = platform.BuildEmuPlatform()
	}
	r.GPUDriver.AcceptHook(r.KernelTimeCounter)
	r.GPUDriver.Run()
}

func (r *Runner) parseGPUFlag() {
	r.gpuIDs = make([]int, 0)
	gpuIDTokens := strings.Split(*gpuFlag, ",")
	for _, t := range gpuIDTokens {
		gpuID, err := strconv.Atoi(t)
		if err != nil {
			log.Fatal(err)
		}
		r.gpuIDs = append(r.gpuIDs, gpuID)
	}
}

// AddBenchmark adds an benchmark that the driver runs
func (r *Runner) AddBenchmark(b benchmarks.Benchmark) {
	b.SelectGPU(r.gpuIDs)
	r.Benchmarks = append(r.Benchmarks, b)
}

func (r *Runner) AddBenchmarkWithoutSettingGPUsToUse(b benchmarks.Benchmark) {
	r.Benchmarks = append(r.Benchmarks, b)
}

// Run runs the benchmark on the simulator
func (r *Runner) Run() {
	var wg sync.WaitGroup
	for _, b := range r.Benchmarks {
		wg.Add(1)
		go func(b benchmarks.Benchmark, wg *sync.WaitGroup) {
			b.Run()
			if *verify {
				b.Verify()
			}
			wg.Done()
		}(b, &wg)
	}
	wg.Wait()

	r.Engine.Finished()
	fmt.Printf("Kernel time: %.12f\n", r.KernelTimeCounter.TotalTime)
	fmt.Printf("Total time: %.12f\n", r.Engine.CurrentTime())
}
