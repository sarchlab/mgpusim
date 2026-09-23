package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/emptykernel"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var numBlocks = flag.Int("num-blocks", 1,
	"Number of work-groups in the empty grid (scaling axis).")
var blockSize = flag.Int("block-size", 256,
	"Work-items per work-group.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := emptykernel.NewBenchmark(runner.Driver())
	benchmark.NumBlocks = *numBlocks
	benchmark.BlockSize = *blockSize
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
