package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/sharedmemlatency"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var numBlocks = flag.Int("num-blocks", 16,
	"The number of work-groups (grid X dimension).")
var innerIters = flag.Int("inner-iters", 8,
	"The number of outer timing iterations the kernel performs.")
var blockSize = flag.Int("block-size", 256,
	"Threads per work-group (block size).")
var accessPattern = flag.String("access-pattern", "no_conflict",
	"Which kernel(s) to run: no_conflict | conflict | both.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := sharedmemlatency.NewBenchmark(runtime)
	benchmark.NumBlocks = *numBlocks
	benchmark.InnerIters = *innerIters
	benchmark.BlockSize = *blockSize
	benchmark.AccessPattern = *accessPattern
	benchmark.Arch = config.ArchType

	config.Configure(benchmark)
	runner.AddBenchmark(benchmark)

	if err := simulation.Initialize(); err != nil {
		panic(err)
	}
	if err := runner.Run(); err != nil {
		panic(err)
	}
	if err := finish(); err != nil {
		panic(err)
	}
}
