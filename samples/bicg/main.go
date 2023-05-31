package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v3/benchmarks/polybench/bicg"
	"github.com/sarchlab/mgpusim/v3/samples/runner"
)

var xFlag = flag.Int("x", 4096, "The width of the matrix.")
var yFlag = flag.Int("y", 4096, "The height of the matrix.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := bicg.NewBenchmark(runner.Driver())
	benchmark.NX = *xFlag
	benchmark.NY = *yFlag

	runner.AddBenchmark(benchmark)

	runner.Run()
}
