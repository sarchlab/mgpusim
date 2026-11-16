package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/fp16throughput"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var fmasPerThread = flag.Int("fmas-per-thread", 64,
	"The number of __hfma2 FMAs each thread performs (rounded to a multiple of 4).")
var numBlocks = flag.Int("num-blocks", 2, "The number of work-groups to launch.")
var threadsPerBlock = flag.Int("threads-per-block", 64,
	"The number of work-items per work-group.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := fp16throughput.NewBenchmark(runtime)
	benchmark.FmasPerThread = *fmasPerThread
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
