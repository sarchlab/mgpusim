package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/shoc/stencil2d"
	"gitlab.com/akita/gcn3/samples/runner"
)

// var numData = flag.Int("length", 4096, "The number of samples to filter.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := stencil2d.NewBenchmark(runner.GPUDriver)
	benchmark.NumIteration = 1
	benchmark.NumRows = 64
	benchmark.NumCols = 64

	runner.AddBenchmark(benchmark)

	runner.Run()
}
