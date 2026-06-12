package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/amdappsdk/floydwarshall"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numNodes = flag.Int("node", 16, "The number of nodes in the graph")
var numIterations = flag.Int("iter", 0,
	`The number of iterations to run. If this value is set to 0 or a number
	larger than the number of nodes, it will be reset to the number of nodes.`)

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := floydwarshall.NewBenchmark(runner.Driver())
	benchmark.NumNodes = uint32(*numNodes)
	benchmark.NumIterations = uint32(*numIterations)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
