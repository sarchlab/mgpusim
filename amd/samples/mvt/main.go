package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/polybench/mvt"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var nFlag = flag.Int("n", 4096, "The dimension of the matrix.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := mvt.NewBenchmark(runner.Driver())
	benchmark.N = *nFlag

	runner.AddBenchmark(benchmark)

	runner.Run()
}
