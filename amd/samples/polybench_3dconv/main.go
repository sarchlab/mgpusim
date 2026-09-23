package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/polybench/conv3d"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var n = flag.Int("size", 32, "The volume edge length N (NxNxN volume).")
var filterSize = flag.Int("filter-size", 3, "The cubic filter edge length.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := conv3d.NewBenchmark(runner.Driver())
	benchmark.N = *n
	benchmark.FilterSize = *filterSize
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
