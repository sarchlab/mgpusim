package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/polybench/jacobi2d"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var n = flag.Int("size", 64, "The grid dimension N (N×N grid).")
var tsteps = flag.Int("tsteps", 10, "The number of Jacobi time steps.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := jacobi2d.NewBenchmark(runner.Driver())
	benchmark.N = *n
	benchmark.TSteps = *tsteps
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
