package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/parboil/cutcp"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numAtoms = flag.Int("num_atoms", 4096, "Number of atoms")
var gridSize = flag.Int("grid_size", 32, "Grid size (each dimension)")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := cutcp.NewBenchmark(runner.Driver())
	benchmark.NumAtoms = *numAtoms
	benchmark.GridSize = *gridSize

	runner.AddBenchmark(benchmark)

	runner.Run()
}
