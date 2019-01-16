package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/heteromark/fir"
	"gitlab.com/akita/gcn3/samples/runner"
)

var numData = flag.Int("length", 4096, "The number of samples to filter.")

func main() {
	flag.Parse()

	runner := runner.Runner{}
	runner.Init()

	benchmark := fir.NewBenchmark(runner.GPUDriver)
	benchmark.Length = *numData
	runner.Benchmark = benchmark

	runner.Run()
}
