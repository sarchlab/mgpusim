package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/amdappsdk/matrixtranspose"
	"gitlab.com/akita/gcn3/samples/runner"
)

var dataWidth = flag.Int("width", 256, "The dimension of the square matrix.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := matrixtranspose.NewBenchmark(runner.GPUDriver)
	benchmark.Width = *dataWidth

	runner.AddBenchmark(benchmark)

	runner.Run()
}
