package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/polybench/gesummv"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var nFlag = flag.Int("n", 4096, "The dimension of the matrices.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := gesummv.NewBenchmark(runner.Driver())
	benchmark.N = *nFlag

	runner.AddBenchmark(benchmark)

	runner.Run()
}
