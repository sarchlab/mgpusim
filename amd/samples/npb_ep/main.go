package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/npb/ep"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var n = flag.Int("size", 4096, "The number of Gaussian pairs (work-items).")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := ep.NewBenchmark(runner.Driver())
	benchmark.N = *n
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
