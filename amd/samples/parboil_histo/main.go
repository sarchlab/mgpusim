// Package main runs the Parboil Histogram benchmark.
package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/parboil/histo"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numElements = flag.Int("num_elements", 1048576, "Number of input elements")
var numBins = flag.Int("num_bins", 256, "Number of histogram bins")

func main() {
	flag.Parse()

	r := new(runner.Runner).Init()

	benchmark := histo.NewBenchmark(r.Driver())
	benchmark.NumElements = *numElements
	benchmark.NumBins = *numBins

	r.AddBenchmark(benchmark)
	r.Run()
}
