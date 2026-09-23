package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/mixbench"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var numElements = flag.Int("size", 4096,
	"The number of float elements (one work-item per element).")
var numFmas = flag.Int("fmas", 16,
	"The FP32 FMA-chain length per work-item.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := mixbench.NewBenchmark(runner.Driver())
	benchmark.NumElements = *numElements
	benchmark.NumFmas = *numFmas
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
