package main

import (
	"flag"

	"gitlab.com/akita/mgpusim/benchmarks/amdappsdk/nbody"
	"gitlab.com/akita/mgpusim/samples/runner"
)

var numIter = flag.Int("iter", 8, "The number of iterations to run.")
var particles = flag.Int("particles", 1024, "The number of particles in the body.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := nbody.NewBenchmark(runner.GPUDriver)
	benchmark.NumIterations = int32(*numIter)
	benchmark.NumParticles = int32(*particles)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
