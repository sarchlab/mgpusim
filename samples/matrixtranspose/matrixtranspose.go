package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/amdappsdk/matrixtranspose"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/platform"
)

var (
	gpuDriver *driver.Driver
	benchmark *matrixtranpose.Benchmark
)

var timing = flag.Bool("timing", false, "Run detailed timing simulation.")
var parallel = flag.Bool("parallel", false, "Run the simulation in parallel.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var instTracing = flag.Bool("trace-inst", false, "Generate instruction trace for visualization purposes.")
var verify = flag.Bool("verify", false, "Verify the emulation result.")
var memTracing = flag.Bool("trace-mem", false, "Generate memory trace")
var dataWidth = flag.Int("width", 256, "The dimension of the square matrix.")

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
	benchmark = matrixtranpose.NewBenchmark(gpuDriver)
	benchmark.Width = *dataWidth
}
