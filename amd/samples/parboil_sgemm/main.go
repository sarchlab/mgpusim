package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/parboil/sgemm"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var n = flag.Int("size", 128, "The matrix dimension N (N×N matrices).")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := sgemm.NewBenchmark(runner.Driver())
	benchmark.N = *n
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
