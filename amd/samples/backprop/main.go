package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/backprop"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var size = flag.Int("size", 65536, "The number of input units")
var skipLayerforward = flag.Bool("skip-layerforward", false,
	"Skip the layerforward kernel and run only adjust_weights")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := backprop.NewBenchmark(runner.Driver())
	benchmark.NumInput = *size
	benchmark.SkipLayerforward = *skipLayerforward

	runner.AddBenchmark(benchmark)

	runner.Run()
}
