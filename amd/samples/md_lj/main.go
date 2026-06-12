package main

import (
	"flag"

	mdlj "github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/md_lj"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numElements = flag.Int("num_elements", 4096, "Number of atoms")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := mdlj.NewBenchmark(runner.Driver())
	benchmark.SetNumElements(*numElements)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
