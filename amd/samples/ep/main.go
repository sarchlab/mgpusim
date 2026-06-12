package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/heteromark/ep"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numElements = flag.Int("elements", 1048576,
	"Number of elements for exclusive prefix scan")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := ep.NewBenchmark(runner.Driver())
	benchmark.NumElements = *numElements

	runner.AddBenchmark(benchmark)

	runner.Run()
}
