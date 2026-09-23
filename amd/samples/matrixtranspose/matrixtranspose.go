package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/amdappsdk/matrixtranspose"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var dataWidth = flag.Int("width", 256, "The dimension of the square matrix.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := matrixtranspose.NewBenchmark(runner.Driver())
	benchmark.Width = *dataWidth
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
