package main

import (
	"flag"

	memlatencychase "github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/mem_latency_chase"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var (
	numElements = flag.Int("num_elements", 1048576, "Number of elements")
	numChases   = flag.Int("num_chases", 0, "Number of pointer-chase hops; 0 uses benchmark defaults")
)

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := memlatencychase.NewBenchmark(runner.Driver())
	benchmark.SetNumElements(*numElements)
	if *numChases > 0 {
		benchmark.SetNumChases(*numChases)
	}

	runner.AddBenchmark(benchmark)

	runner.Run()
}
