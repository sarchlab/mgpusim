package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/backprop"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var size = flag.Int("size", 65536, "The number of input units")

// This binary always runs the backprop benchmark with the layerforward
// kernel skipped, leaving adjust_weights as the only kernel timed. It
// exists so the CI matrix can build a separate binary dedicated to
// measuring adjust_weights in isolation.
func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := backprop.NewBenchmark(runner.Driver())
	benchmark.NumInput = *size
	benchmark.SkipLayerforward = true

	runner.AddBenchmark(benchmark)

	runner.Run()
}
