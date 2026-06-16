package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/fp16throughput"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var fmasPerThread = flag.Int("fmas-per-thread", 64,
	"The number of __hfma2 FMAs each thread performs (rounded to a multiple of 4).")
var numBlocks = flag.Int("num-blocks", 2, "The number of work-groups to launch.")
var threadsPerBlock = flag.Int("threads-per-block", 64,
	"The number of work-items per work-group.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := fp16throughput.NewBenchmark(runner.Driver())
	benchmark.FmasPerThread = *fmasPerThread
	benchmark.NumBlocks = *numBlocks
	benchmark.ThreadsPerBlock = *threadsPerBlock
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
