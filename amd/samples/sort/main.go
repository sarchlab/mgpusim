package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/shoc/sort"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numElements = flag.Int("num_elements", 65536, "Number of elements")

func main() {
	flag.Parse()

	r := new(runner.Runner).Init()

	benchmark := sort.NewBenchmark(r.Driver())
	benchmark.SetNumElements(*numElements)

	r.AddBenchmark(benchmark)

	r.Run()
}
