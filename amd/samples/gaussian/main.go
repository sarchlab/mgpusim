package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/gaussian"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var size = flag.Int("size", 256, "Matrix size (N×N)")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := gaussian.NewBenchmark(runner.Driver())
	benchmark.SetSize(*size)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
