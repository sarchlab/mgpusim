package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/shoc/scan"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numElements = flag.Int("num_elements", 1048576, "Number of elements")

func main() {
	flag.Parse()

	r := new(runner.Runner).Init()

	benchmark := scan.NewBenchmark(r.Driver())
	benchmark.SetNumElements(*numElements)

	r.AddBenchmark(benchmark)

	r.Run()
}
