package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/dnn/maxpooling"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/platform"
)

var (
	gpuDriver *driver.Driver
	benchmark *maxpooling.Benchmark
)

var timing = flag.Bool("timing", false, "Run detailed timing simulation.")
var parallel = flag.Bool("parallel", false, "Run the simulation in parallel.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var instTracing = flag.Bool("trace-inst", false, "Generate instruction trace for visualization purposes.")
var verify = flag.Bool("verify", false, "Verify the emulation result.")
var memTracing = flag.Bool("trace-mem", false, "Generate memory trace")
var n = flag.Int("n", 1, "Batch size.")
var c = flag.Int("c", 1, "Channel size.")
var h = flag.Int("h", 64, "Height.")
var w = flag.Int("w", 64, "Weight.")

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
		_, _, gpuDriver, _ = platform.BuildR9NanoPlatform()
	} else {
		_, _, gpuDriver, _ = platform.BuildEmuPlatform()
	}
}

func initBenchmark() {
	benchmark = maxpooling.NewBenchmark(gpuDriver, n, c, h, w)
	benchmark.N = *n
	benchmark.C = *c
	benchmark.H = *h
	benchmark.W = *w
}
