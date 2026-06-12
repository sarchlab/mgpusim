package main

import (
	"flag"

	branchdiv50pct "github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/branch_div_50pct"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numElements = flag.Int("num_elements", 1048576, "Number of elements")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := branchdiv50pct.NewBenchmark(runner.Driver())
	benchmark.SetNumElements(*numElements)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
