package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/rodinia/hotspot3d"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var gridSize = flag.Int("size", 32,
	"The edge length N of the NxNxN temperature grid.")
var numIterations = flag.Int("iterations", 2,
	"The number of stencil time-steps to run.")
var ambTemp = flag.Float64("amb-temp", 80.0,
	"The ambient temperature.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := hotspot3d.NewBenchmark(runtime)
	benchmark.GridSize = *gridSize
	benchmark.NumIterations = *numIterations
	benchmark.AmbTemp = float32(*ambTemp)
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
