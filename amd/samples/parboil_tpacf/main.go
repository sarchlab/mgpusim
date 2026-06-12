// Package main runs the Parboil TPACF benchmark.
package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/parboil/tpacf"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numPoints = flag.Int("num_points", 4096, "Number of sky catalog points")
var numBins = flag.Int("num_bins", 64, "Number of histogram bins")

func main() {
	flag.Parse()

	r := new(runner.Runner).Init()

	benchmark := tpacf.NewBenchmark(r.Driver())
	benchmark.NumPoints = *numPoints
	benchmark.NumBins = *numBins

	r.AddBenchmark(benchmark)
	r.Run()
}
