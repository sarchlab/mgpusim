package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/amdappsdk/bitonicsort"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/platform"
)

var (
	gpuDriver *driver.Driver
	benchmark *bitonicsort.Benchmark
)

var kernelFilePath = flag.String(
	"kernel file path",
	"kernels.hsaco",
	"The path to the kernel hsaco file.",
)
var timing = flag.Bool("timing", false, "Run detailed timing simulation.")
var parallel = flag.Bool("parallel", false, "Run the simulation in parallel.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var instTracing = flag.Bool("trace-inst", false, "Generate instruction trace for visualization purposes.")
var verify = flag.Bool("verify", false, "Verify the emulation result.")
var memTracing = flag.Bool("trace-mem", false, "Generate memory trace")
var length = flag.Int("length", 1024, "The length of array to sort.")
var orderAscending = flag.Bool("order-asc", true, "Sorting in ascending order.")

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
		_, gpuDriver = platform.BuildNR9NanoPlatform(4)
	} else {
		_, _, gpuDriver, _ = platform.BuildEmuPlatform()
	}
}

func initBenchmark() {
	benchmark = bitonicsort.NewBenchmark(gpuDriver)
	benchmark.Length = *length
	benchmark.OrderAscending = *orderAscending
}
