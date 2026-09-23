package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/rodinia/pathfinder"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var rows = flag.Int("rows", 64, "The number of grid rows (DP sweep iterations).")
var cols = flag.Int("cols", 128, "The number of grid columns (row width).")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := pathfinder.NewBenchmark(runner.Driver())
	benchmark.Rows = *rows
	benchmark.Cols = *cols
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
