package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/heteromark/pagerank"
	"gitlab.com/akita/gcn3/samples/runner"
)

var numNodes = flag.Int("nodes", 16, "The number of nodes")
var numConnections = flag.Int("connections", 8, "The number of connections")
var maxIterations = flag.Int("iterations", 16, "The number of iterations")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := pagerank.NewBenchmark(runner.GPUDriver)
	benchmark.NumNodes = uint32(*numNodes)
	benchmark.NumConnections = uint32(*numConnections)
	benchmark.MaxIterations = uint32(*maxIterations)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
