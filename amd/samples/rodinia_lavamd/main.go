package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/rodinia/lavamd"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var numBoxes = flag.Int("num-boxes", 4,
	"The number of boxes per dimension (grid is num-boxes^3 boxes).")
var particlesPerBox = flag.Int("particles-per-box", 100,
	"The number of particles in each box.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := lavamd.NewBenchmark(runtime)
	benchmark.NumBoxes = *numBoxes
	benchmark.ParticlesPerBox = *particlesPerBox
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
