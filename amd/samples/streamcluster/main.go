package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/streamcluster"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numPoints = flag.Int("num_points", 256, "Number of points")
var numDimensions = flag.Int("num_dimensions", 32, "Number of dimensions")
var numCenters = flag.Int("num_centers", 10, "Number of centers")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := streamcluster.NewBenchmark(runner.Driver())
	benchmark.SetNumPoints(*numPoints)
	benchmark.SetDim(*numDimensions)
	benchmark.SetNumCenters(*numCenters)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
