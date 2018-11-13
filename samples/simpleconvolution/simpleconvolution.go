package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/amdappsdk/simpleconvolution"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/platform"
)

var (
	gpuDriver *driver.Driver
	benchmark *simpleconvolution.Benchmark
)

var timing = flag.Bool("timing", false, "Run detailed timing simulation.")
var parallel = flag.Bool("parallel", false, "Run the simulation in parallel.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var memTracing = flag.Bool("trace-mem", false, "Generate memory trace")
var instTracing = flag.Bool("trace-inst", false, "Generate instruction trace for visualization purposes.")
var verify = flag.Bool("verify", false, "Verify the emulation result.")
var widthFlag = flag.Uint("width", 254, "The width of the input matrix.")
var heightFlag = flag.Uint("height", 254, "The height of the input matrix.")
var maskSizeFlag = flag.Uint("mask-size", 3, "The size of the mask.")

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
	benchmark = simpleconvolution.NewBenchmark(gpuDriver)
	benchmark.Width = uint32(*widthFlag)
	benchmark.Height = uint32(*heightFlag)
	benchmark.SetMaskSize(uint32(*maskSizeFlag))
}
