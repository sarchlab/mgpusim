package main

import (
	"flag"

	sfunsin "github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/sfun_sin"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numOps = flag.Int("num_ops", 1048576, "Number of loop iterations per thread")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := sfunsin.NewBenchmark(runner.Driver())
	benchmark.SetNumOps(*numOps)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
