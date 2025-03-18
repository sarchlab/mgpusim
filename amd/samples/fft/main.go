package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/shoc/fft"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var mb = flag.Int("MB", 8, "data size (in megabytes)")
var passes = flag.Int("passes", 2, "number of passes")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := fft.NewBenchmark(runner.Driver())
	benchmark.Bytes = int32(*mb)
	benchmark.Passes = int32(*passes)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
