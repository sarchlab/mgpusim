package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/int32throughput"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var madsPerThread = flag.Int("mads", 4096,
	"The number of int32 multiply-add operations per thread "+
		"(rounded down to a multiple of 4).")
var numBlocks = flag.Int("blocks", 16,
	"The number of work-groups (blocks) in the 1D grid.")
var threadsPerBlock = flag.Int("threads-per-block", 256,
	"Work-group size (threads per block).")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := int32throughput.NewBenchmark(runtime)
	benchmark.MadsPerThread = *madsPerThread
	benchmark.NumBlocks = *numBlocks
	benchmark.ThreadsPerBlock = *threadsPerBlock
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
