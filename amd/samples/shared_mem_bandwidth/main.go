package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/sharedmembandwidth"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
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

	runner := new(runner.Runner).Init()

	benchmark := sharedmembandwidth.NewBenchmark(runner.Driver())
	benchmark.NumBlocks = *numBlocks
	benchmark.InnerIters = *innerIters
	benchmark.BlockSize = *blockSize
	benchmark.AccessPattern = *accessPattern
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
