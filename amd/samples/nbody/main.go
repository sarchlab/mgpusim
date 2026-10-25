package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/amdappsdk/nbody"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var numIter = flag.Int("iter", 8, "The number of iterations to run.")
var particles = flag.Int("particles", 1024, "The number of particles in the body.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := nbody.NewBenchmark(runtime)
	benchmark.Arch = config.ArchType
	benchmark.NumIterations = int32(*numIter)
	benchmark.NumParticles = int32(*particles)

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
