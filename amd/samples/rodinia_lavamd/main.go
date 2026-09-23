package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/rodinia/lavamd"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var numBoxes = flag.Int("num-boxes", 4,
	"The number of boxes per dimension (grid is num-boxes^3 boxes).")
var particlesPerBox = flag.Int("particles-per-box", 100,
	"The number of particles in each box.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := lavamd.NewBenchmark(runner.Driver())
	benchmark.NumBoxes = *numBoxes
	benchmark.ParticlesPerBox = *particlesPerBox
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
