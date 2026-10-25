package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/fp32throughput"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var numBlocks = flag.Int("num-blocks", 4, "The number of thread blocks to launch.")
var fmasPerThread = flag.Int("fmas", 256,
	"The number of FMA iterations per thread (rounded down to a multiple of 4).")
var threadsPerBlock = flag.Int("threads-per-block", 256,
	"The work-group (block) size; must be <= 1024.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := fp32throughput.NewBenchmark(runtime)
	benchmark.NumBlocks = *numBlocks
	benchmark.FmasPerThread = *fmasPerThread
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
