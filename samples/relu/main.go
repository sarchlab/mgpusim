package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/dnn/relu"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/platform"
)

var (
	gpuDriver *driver.Driver
	benchmark *relu.Benchmark
)

var timing = flag.Bool("timing", false, "Run detailed timing simulation.")
var parallel = flag.Bool("parallel", false, "Run the simulation in parallel.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var instTracing = flag.Bool("trace-inst", false, "Generate instruction trace for visualization purposes.")
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

}

func configure() {
	flag.Parse()

	if *parallel {
		platform.UseParallelEngine = true
	}

	if *isaDebug {
		platform.DebugISA = true
	}

	if *instTracing {
		platform.TraceInst = true
	}

	if *memTracing {
		platform.TraceMem = true
	}
}

func initPlatform() {
	if *timing {
		_, _, gpuDriver = platform.BuildR9NanoPlatform()
	} else {
		_, _, gpuDriver, _ = platform.BuildEmuPlatform()
	}
}

func initBenchmark() {
	benchmark = relu.NewBenchmark(gpuDriver)
	benchmark.Length = *numData
}
