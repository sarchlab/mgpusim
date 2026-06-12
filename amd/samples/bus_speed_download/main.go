package main

import (
	"flag"

	busspeeddownload "github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/bus_speed_download"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numElements = flag.Int("num_elements", 1048576, "Number of elements")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := busspeeddownload.NewBenchmark(runner.Driver())
	benchmark.SetNumElements(*numElements)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
