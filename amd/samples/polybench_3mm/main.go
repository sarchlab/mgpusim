package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/polybench/threemm"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var n = flag.Int("size", 64,
	"The matrix dimension N (all matrices are N×N).")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := threemm.NewBenchmark(runner.Driver())
	benchmark.NI = *n
	benchmark.NJ = *n
	benchmark.NK = *n
	benchmark.NL = *n
	benchmark.NM = *n
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
