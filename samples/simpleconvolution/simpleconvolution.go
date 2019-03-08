package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/amdappsdk/simpleconvolution"
	"gitlab.com/akita/gcn3/samples/runner"
)

var widthFlag = flag.Uint("width", 254, "The width of the input matrix.")
var heightFlag = flag.Uint("height", 254, "The height of the input matrix.")
var maskSizeFlag = flag.Uint("mask-size", 3, "The size of the mask.")

func main() {
	flag.Parse()

	runner := runner.Runner{}
	runner.Init()

	benchmark := simpleconvolution.NewBenchmark(runner.GPUDriver)
	benchmark.Height = uint32(*heightFlag)
	benchmark.Width = uint32(*widthFlag)
	benchmark.SetMaskSize(uint32(*maskSizeFlag))

	runner.AddBenchmark(benchmark)

	runner.Run()
}
