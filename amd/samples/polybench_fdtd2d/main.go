package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/polybench/fdtd2d"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var n = flag.Int("size", 16, "The grid dimension N (N×N field arrays).")
var tmax = flag.Int("tmax", 10, "The number of FDTD time steps.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := fdtd2d.NewBenchmark(runner.Driver())
	benchmark.N = *n
	benchmark.TMax = *tmax
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
