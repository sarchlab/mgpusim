package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/babelstream"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var n = flag.Int("size", 4096, "The vector length N.")
var scalar = flag.Float64("scalar", 2.0, "The scalar used by scale and triad.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := babelstream.NewBenchmark(runtime)
	benchmark.N = *n
	benchmark.Scalar = float32(*scalar)
	benchmark.Arch = config.ArchType

	config.Configure(benchmark)
	runner.AddBenchmark(benchmark)

	if err := simulation.Initialize(); err != nil {
		panic(err)
	}
	if err := runner.Run(); err != nil {
		panic(err)
	}
	if err := finish(); err != nil {
		panic(err)
	}
}
