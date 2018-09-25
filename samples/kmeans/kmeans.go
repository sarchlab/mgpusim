package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/kmeans"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/platform"
)

var (
	gpuDriver *driver.Driver
	benchmark *kmeans.Benchmark
)

var timing = flag.Bool("timing", false, "Run detailed timing simulation.")
var parallel = flag.Bool("parallel", false, "Run the simulation in parallel.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var instTracing = flag.Bool("trace-inst", false, "Generate instruction trace for visualization purposes.")
var verify = flag.Bool("verify", false, "Verify the emulation result.")
var memTracing = flag.Bool("trace-mem", false, "Generate memory trace")
var points = flag.Int("points", 4096, "The number of points.")
var clusters = flag.Int("clusters", 5, "The number of clusters.")
var features = flag.Int("features", 32, "The number of features for each point.")
var maxIter = flag.Int("max-iter", 20, "The maximum number of iterations to run")

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
	benchmark = kmeans.NewBenchmark(gpuDriver)
	benchmark.NumPoints = *points
	benchmark.NumFeatures = *features
	benchmark.NumClusters = *clusters
	benchmark.MaxIter = *maxIter
}
