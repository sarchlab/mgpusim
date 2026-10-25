package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/rodinia/backprop"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var (
	inputN  = flag.Int("input", 64, "Number of input units.")
	hiddenN = flag.Int("hidden", 32, "Number of hidden units.")
	outputN = flag.Int("output", 4, "Number of output units.")
)

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := backprop.NewBenchmark(runtime)
	benchmark.InputN = *inputN
	benchmark.HiddenN = *hiddenN
	benchmark.OutputN = *outputN
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
