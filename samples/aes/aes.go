package main

import (
	"flag"

	"gitlab.com/akita/mgpusim/v3/benchmarks/heteromark/aes"
	"gitlab.com/akita/mgpusim/v3/samples/runner"
)

var lenInput = flag.Int("length", 65536, "The length of array to sort.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := aes.NewBenchmark(runner.Driver())
	benchmark.Length = *lenInput

	runner.AddBenchmark(benchmark)

	runner.Run()
}
