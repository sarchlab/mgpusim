package main

import (
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/heteromark/fir"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numData = runner.BenchmarkFlags.Int("length", 4096, "The number of samples to filter.")

func main() {
	// flag.Parse() // This is no longer needed as runner.Init() calls ParseAllFlags()

	runner := new(runner.Runner).Init()

	benchmark := fir.NewBenchmark(runner.Driver())
	benchmark.Length = *numData

	runner.AddBenchmark(benchmark)

	runner.Run()
}
