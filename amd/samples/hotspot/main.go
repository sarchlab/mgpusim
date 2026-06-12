package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/hotspot"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var gridSize = flag.Int("grid_size", 512, "Grid size (rows = cols = grid_size)")
var numIterations = flag.Int("num_iterations", 10, "Number of simulation iterations")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := hotspot.NewBenchmark(runner.Driver())
	benchmark.SetSize(*gridSize)
	benchmark.SetNumIterations(*numIterations)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
