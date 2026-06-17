package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/altis/cfd"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var n = flag.Int("size", 256, "The number of mesh cells N.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := cfd.NewBenchmark(runner.Driver())
	benchmark.N = *n
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
