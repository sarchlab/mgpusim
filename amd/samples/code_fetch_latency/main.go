package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/codefetchlatency"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var bodyOps = flag.Int("body-ops", 256,
	"The number of FMAs in the loop body; a power of two from 16 to 65536.")
var loops = flag.Int("loops", 1, "How many times the kernel executes its body.")
var numBlocks = flag.Int("num-blocks", 1, "The number of thread blocks to launch.")
var threadsPerBlock = flag.Int("threads-per-block", 64,
	"The work-group (block) size; must be <= 1024.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := codefetchlatency.NewBenchmark(runtime)
	benchmark.BodyOps = *bodyOps
	benchmark.Loops = *loops
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
