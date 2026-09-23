package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/rodinia/hotspot3d"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var gridSize = flag.Int("size", 32,
	"The edge length N of the NxNxN temperature grid.")
var numIterations = flag.Int("iterations", 2,
	"The number of stencil time-steps to run.")
var ambTemp = flag.Float64("amb-temp", 80.0,
	"The ambient temperature.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := hotspot3d.NewBenchmark(runner.Driver())
	benchmark.GridSize = *gridSize
	benchmark.NumIterations = *numIterations
	benchmark.AmbTemp = float32(*ambTemp)
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
