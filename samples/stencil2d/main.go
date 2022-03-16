package main

import (
	"flag"

	"gitlab.com/akita/mgpusim/v3/benchmarks/shoc/stencil2d"
	"gitlab.com/akita/mgpusim/v3/samples/runner"
)

var numRow = flag.Int("row", 64, "The number of rows in the input matrix.")
var numCol = flag.Int("col", 64, "The number of columns in the input matrix.")
var numIter = flag.Int("iter", 5, "The number of iterations to run.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := stencil2d.NewBenchmark(runner.Driver())
	benchmark.NumIteration = *numIter
	benchmark.NumRows = *numRow + 2
	benchmark.NumCols = *numCol + 2

	runner.AddBenchmark(benchmark)

	runner.Run()
}
