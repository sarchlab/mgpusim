package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/polybench/correlation"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var n = flag.Int("size", 64,
	"The data matrix dimension N (N×N matrix, M = N samples).")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := correlation.NewBenchmark(runner.Driver())
	benchmark.N = *n
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
