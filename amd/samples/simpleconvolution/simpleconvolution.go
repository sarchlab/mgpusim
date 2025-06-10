package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/amdappsdk/simpleconvolution"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var widthFlag = flag.Uint("width", 254, "The width of the input matrix.")
var heightFlag = flag.Uint("height", 254, "The height of the input matrix.")
var maskSizeFlag = flag.Uint("mask-size", 3, "The size of the mask.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := simpleconvolution.NewBenchmark(runner.Driver())
	benchmark.Height = uint32(*heightFlag)
	benchmark.Width = uint32(*widthFlag)
	benchmark.SetMaskSize(uint32(*maskSizeFlag))

	runner.AddBenchmark(benchmark)

	runner.Run()
}
