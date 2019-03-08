package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/dnn/relu"
	"gitlab.com/akita/gcn3/samples/runner"
)

var numData = flag.Int("length", 4096, "The number of samples to filter.")

func main() {
	flag.Parse()

	runner := runner.Runner{}
	runner.Init()

	benchmark := relu.NewBenchmark(runner.GPUDriver)
	benchmark.Length = *numData

	runner.AddBenchmark(benchmark)

	runner.Run()
}
