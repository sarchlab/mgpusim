package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/heteromark/ga"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var populationSize = flag.Int("population-size", 1024,
	"The population size")
var maxGenerations = flag.Int("max-generations", 100,
	"The maximum number of generations")
var mutationRate = flag.Float64("mutation-rate", 0.01,
	"The mutation rate")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := ga.NewBenchmark(runner.Driver())
	benchmark.PopulationSize = *populationSize
	benchmark.MaxGenerations = *maxGenerations
	benchmark.MutationRate = float32(*mutationRate)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
