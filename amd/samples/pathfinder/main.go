package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/pathfinder"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var cols = flag.Int("cols", 512, "Number of columns")
var rows = flag.Int("rows", 100, "Number of rows")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := pathfinder.NewBenchmark(runner.Driver())
	benchmark.SetCols(*cols)
	benchmark.SetRows(*rows)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
