package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/tango/binomialoptions"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var numOptions = flag.Int("options", 8, "The number of options to price.")
var numSteps = flag.Int("steps", 64,
	"The number of binomial tree steps per option (steps+1 must be <= 256).")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := binomialoptions.NewBenchmark(runner.Driver())
	benchmark.NumOptions = *numOptions
	benchmark.NumSteps = *numSteps
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
