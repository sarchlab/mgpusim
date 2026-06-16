package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/rodinia/hotspot"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var gridSize = flag.Int("size", 32,
	"The grid dimension N (the thermal grid is N×N).")
var numIterations = flag.Int("iterations", 10,
	"The number of stencil time-steps to simulate.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := hotspot.NewBenchmark(runner.Driver())
	benchmark.GridSize = *gridSize
	benchmark.NumIterations = *numIterations
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
