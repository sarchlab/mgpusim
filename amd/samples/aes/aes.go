package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/heteromark/aes"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var lenInput = flag.Int("length", 65536, "The length of array to sort.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := aes.NewBenchmark(runner.Driver())
	benchmark.Length = *lenInput

	runner.AddBenchmark(benchmark)

	runner.Run()
}
