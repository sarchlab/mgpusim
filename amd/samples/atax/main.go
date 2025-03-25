package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/polybench/atax"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var xFlag = flag.Int("x", 4096, "The width of the matrix.")
var yFlag = flag.Int("y", 4096, "The height of the matrix.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := atax.NewBenchmark(runner.Driver())
	benchmark.NX = *xFlag
	benchmark.NY = *yFlag

	runner.AddBenchmark(benchmark)

	runner.Run()
}
