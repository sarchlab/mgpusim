package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/rodinia/gaussian"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var n = flag.Int("size", 64, "The matrix dimension N (N×N system Ax=b).")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := gaussian.NewBenchmark(runner.Driver())
	benchmark.N = *n
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
