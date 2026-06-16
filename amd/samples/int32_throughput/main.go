package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/int32throughput"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var madsPerThread = flag.Int("mads", 4096,
	"The number of int32 multiply-add operations per thread "+
		"(rounded down to a multiple of 4).")
var numBlocks = flag.Int("blocks", 16,
	"The number of work-groups (blocks) in the 1D grid. "+
		"The block size is fixed at 64.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := int32throughput.NewBenchmark(runner.Driver())
	benchmark.MadsPerThread = *madsPerThread
	benchmark.NumBlocks = *numBlocks
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
