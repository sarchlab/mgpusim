package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/polybench/covariance"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var nFlag = flag.Int("n", 256, "The N dimension (rows).")
var mFlag = flag.Int("m", 256, "The M dimension (columns).")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := covariance.NewBenchmark(runner.Driver())
	benchmark.N = *nFlag
	benchmark.M = *mFlag

	runner.AddBenchmark(benchmark)

	runner.Run()
}
