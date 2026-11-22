package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/rodinia/hotspot"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var gridSize = flag.Int("size", 32,
	"The grid dimension N (the thermal grid is N×N).")
var numIterations = flag.Int("iterations", 10,
	"The number of stencil time-steps to simulate.")

var pyramidHeight = flag.Int("pyramid-height", 1, "Maximum number of stencil steps per launch.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := hotspot.NewBenchmark(runtime)
	benchmark.GridSize = *gridSize
	benchmark.NumIterations = *numIterations
	benchmark.PyramidHeight = *pyramidHeight
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
