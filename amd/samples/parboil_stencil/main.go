package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/parboil/stencil"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var n = flag.Int("size", 32,
	"The grid dimension N (N×N×N grid).")
var numTimesteps = flag.Int("timesteps", 4,
	"The number of stencil time-steps to run.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := stencil.NewBenchmark(runner.Driver())
	benchmark.N = *n
	benchmark.NumTimesteps = *numTimesteps
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
