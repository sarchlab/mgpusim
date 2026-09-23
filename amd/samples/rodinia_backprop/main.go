package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/rodinia/backprop"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var (
	inputN  = flag.Int("input", 64, "Number of input units.")
	hiddenN = flag.Int("hidden", 32, "Number of hidden units.")
	outputN = flag.Int("output", 4, "Number of output units.")
)

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := backprop.NewBenchmark(runner.Driver())
	benchmark.InputN = *inputN
	benchmark.HiddenN = *hiddenN
	benchmark.OutputN = *outputN
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
