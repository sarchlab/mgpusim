package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/polybench/gramschmidt"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var nFlag = flag.Int("n", 256, "The number of columns (N).")
var mFlag = flag.Int("m", 256, "The number of rows (M).")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := gramschmidt.NewBenchmark(runner.Driver())
	benchmark.N = *nFlag
	benchmark.M = *mFlag

	runner.AddBenchmark(benchmark)

	runner.Run()
}
