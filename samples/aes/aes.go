package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/heteromark/aes"
	"gitlab.com/akita/gcn3/samples/runner"
)

var lenInput = flag.Int("length", 65536, "The length of array to sort.")

func main() {
	flag.Parse()

	runner := runner.Runner{}
	runner.Init()

	benchmark := aes.NewBenchmark(runner.GPUDriver)
	benchmark.Length = *lenInput
	runner.Benchmark = benchmark

	runner.Run()
}
