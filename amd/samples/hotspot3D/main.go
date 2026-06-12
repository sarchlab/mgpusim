package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/hotspot3D"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var gridSize = flag.Int("grid_size", 64, "Grid size (nx = ny = nz = grid_size)")
var numIterations = flag.Int("num_iterations", 10, "Number of simulation iterations")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := hotspot3D.NewBenchmark(runner.Driver())
	benchmark.SetGridSize(*gridSize)
	benchmark.SetNumIterations(*numIterations)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
