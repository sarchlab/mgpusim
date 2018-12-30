package main

import (
	"flag"
	"fmt"

	"gitlab.com/akita/gcn3/benchmarks/heteromark/fir"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/platform"
)

var (
	gpuDriver         *driver.Driver
	benchmark         *fir.Benchmark
	kernelTimeCounter *driver.KernelTimeCounter
)

var timing = flag.Bool("timing", false, "Run detailed timing simulation.")
var parallel = flag.Bool("parallel", false, "Run the simulation in parallel.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var visTracing = flag.Bool("trace-vis", false,
	"Generate trace for visualization purposes.")
var verify = flag.Bool("verify", false, "Verify the emulation result.")
var memTracing = flag.Bool("trace-mem", false, "Generate memory trace")
var numData = flag.Int("length", 4096, "The number of samples to filter.")

func main() {
	configure()
	initPlatform()
	initBenchmark()
	benchmark.Run()
	if *verify {
		benchmark.Verify()
	}
	fmt.Printf("Kernel time: %.12f\n", kernelTimeCounter.TotalTime)
}

func configure() {
	flag.Parse()

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
}

func initPlatform() {
	kernelTimeCounter = driver.NewKernelTimeCounter()
	if *timing {
		_, gpuDriver = platform.BuildNR9NanoPlatform(4)
	} else {
		_, _, gpuDriver, _ = platform.BuildEmuPlatform()
	}
	gpuDriver.AcceptHook(kernelTimeCounter)
}

func initBenchmark() {
	benchmark = fir.NewBenchmark(gpuDriver)
	benchmark.Length = *numData
}
