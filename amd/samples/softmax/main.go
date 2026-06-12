package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/softmax"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var rows = flag.Int("rows", 1024, "Number of rows")
var cols = flag.Int("cols", 1024, "Number of columns")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := softmax.NewBenchmark(runner.Driver())
	benchmark.SetRows(*rows)
	benchmark.SetCols(*cols)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
