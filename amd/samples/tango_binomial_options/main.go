package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/tango/binomialoptions"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var numOptions = flag.Int("options", 8, "The number of options to price.")
var numSteps = flag.Int("steps", 64,
	"The number of binomial tree steps per option (steps+1 must be <= 256).")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := binomialoptions.NewBenchmark(runtime)
	benchmark.NumOptions = *numOptions
	benchmark.NumSteps = *numSteps
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
