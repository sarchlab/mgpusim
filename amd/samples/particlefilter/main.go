package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/particlefilter"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numParticles = flag.Int("num_particles", 128, "Number of particles")
var numTimesteps = flag.Int("num_timesteps", 5, "Number of timesteps")
var stateDims = flag.Int("state_dims", 2, "State dimensions")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := particlefilter.NewBenchmark(runner.Driver())
	benchmark.SetNumParticles(*numParticles)
	benchmark.SetNumTimesteps(*numTimesteps)
	benchmark.SetStateDims(*stateDims)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
