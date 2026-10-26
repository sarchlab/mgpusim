package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/amdappsdk/simpleconvolution"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var widthFlag = flag.Uint("width", 254, "The width of the input matrix.")
var heightFlag = flag.Uint("height", 254, "The height of the input matrix.")
var maskSizeFlag = flag.Uint("mask-size", 3, "The size of the mask.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := simpleconvolution.NewBenchmark(runtime)
	benchmark.Height = uint32(*heightFlag)
	benchmark.Width = uint32(*widthFlag)
	benchmark.SetMaskSize(uint32(*maskSizeFlag))
	benchmark.Arch = config.ArchType

	config.Configure(benchmark)
	runner.AddBenchmark(benchmark)

	if err := simulation.Initialize(); err != nil {
		panic(err)
	}
	if err := runner.Run(); err != nil {
		panic(err)
	}
	if err := finish(); err != nil {
		panic(err)
	}
}
