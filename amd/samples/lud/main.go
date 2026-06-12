package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/lud"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var size = flag.Int("size", 256, "The matrix dimension (N×N)")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := lud.NewBenchmark(runner.Driver())
	benchmark.MatrixDim = *size

	runner.AddBenchmark(benchmark)

	runner.Run()
}
