package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/amdappsdk/floydwarshall"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var numNodes = flag.Int("node", 16, "The number of nodes in the graph")
var numIterations = flag.Int("iter", 0,
	`The number of iterations to run. If this value is set to 0 or a number
	larger than the number of nodes, it will be reset to the number of nodes.`)

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := floydwarshall.NewBenchmark(runtime)
	benchmark.NumNodes = uint32(*numNodes)
	benchmark.NumIterations = uint32(*numIterations)
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
