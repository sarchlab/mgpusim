package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/amdappsdk/floydwarshall"
	"gitlab.com/akita/gcn3/samples/runner"
)

var numNodes = flag.Int("nodes", 8, "The number of nodes in the graph")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := floydwarshall.NewBenchmark(runner.GPUDriver)
	benchmark.NumNodes = uint32(*numNodes)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
