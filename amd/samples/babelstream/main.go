package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/babelstream"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var n = flag.Int("size", 4096, "The vector length N.")
var scalar = flag.Float64("scalar", 2.0, "The scalar used by scale and triad.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := babelstream.NewBenchmark(runner.Driver())
	benchmark.N = *n
	benchmark.Scalar = float32(*scalar)
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
