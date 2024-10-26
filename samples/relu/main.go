package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/benchmarks/dnn/layer_benchmarks/relu"
	"github.com/sarchlab/mgpusim/v4/samples/runner"
)

var numData = flag.Int("length", 4096, "The number of samples to filter.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := relu.NewBenchmark(runner.Driver())
	benchmark.Length = *numData

	runner.AddBenchmark(benchmark)

	runner.Run()
}
