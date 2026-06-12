package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/heteromark/hist"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numElements = flag.Int("num-elements", 1048576,
	"The number of input data elements")
var numBins = flag.Int("num-bins", 256,
	"The number of histogram bins")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := hist.NewBenchmark(runner.Driver())
	benchmark.NumElements = *numElements
	benchmark.NumBins = *numBins

	runner.AddBenchmark(benchmark)

	runner.Run()
}
