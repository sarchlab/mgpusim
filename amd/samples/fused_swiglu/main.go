package main

import (
	"flag"

	fusedswiglu "github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/fused_swiglu"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numElements = flag.Int("num_elements", 1572864, "Number of elements")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := fusedswiglu.NewBenchmark(runner.Driver())
	benchmark.SetNumElements(*numElements)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
