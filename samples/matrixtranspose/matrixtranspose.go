package main

import (
	"flag"

	matrixtranpose "gitlab.com/akita/gcn3/benchmarks/amdappsdk/matrixtranspose"
	"gitlab.com/akita/gcn3/samples/runner"
)

var dataWidth = flag.Int("width", 256, "The dimension of the square matrix.")

func main() {
	flag.Parse()

	runner := runner.Runner{}
	runner.Init()

	benchmark := matrixtranpose.NewBenchmark(runner.GPUDriver)
	benchmark.Width = *dataWidth
	runner.Benchmark = benchmark

	runner.Run()
}
