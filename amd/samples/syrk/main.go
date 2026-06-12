package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/polybench/syrk"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var nFlag = flag.Int("n", 256, "The N dimension.")
var mFlag = flag.Int("m", 256, "The M dimension.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := syrk.NewBenchmark(runner.Driver())
	benchmark.N = *nFlag
	benchmark.M = *mFlag

	runner.AddBenchmark(benchmark)

	runner.Run()
}
