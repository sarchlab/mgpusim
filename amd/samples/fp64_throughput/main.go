package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/fp64throughput"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var numBlocks = flag.Int("num-blocks", 4,
	"The number of work-groups (each has 64 work-items).")
var fmasPerThread = flag.Int("fmas-per-thread", 16,
	"The number of FP64 FMA iterations per work-item (rounded to a multiple of 4).")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := fp64throughput.NewBenchmark(runner.Driver())
	benchmark.NumBlocks = *numBlocks
	benchmark.FmasPerThread = *fmasPerThread
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
