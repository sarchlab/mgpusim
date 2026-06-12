package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/lonestar/dmr"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numPoints = flag.Int("num_points", 16384, "Number of initial points")
var maxPassesFlag = flag.Int("max_passes", 3, "Maximum refinement passes")

func main() {
	flag.Parse()

	r := new(runner.Runner).Init()

	benchmark := dmr.NewBenchmark(r.Driver())
	benchmark.NumPoints = *numPoints
	benchmark.MaxPasses = *maxPassesFlag

	r.AddBenchmark(benchmark)
	r.Run()
}
