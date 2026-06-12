package main

import (
	"flag"

	atomicthroughput "github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/atomic_throughput"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numElements = flag.Int("num_elements", 65536, "Number of threads")
var numCounters = flag.Int("num_counters", 1, "Number of atomic counters")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := atomicthroughput.NewBenchmark(runner.Driver())
	benchmark.SetNumElements(*numElements)
	benchmark.SetNumCounters(*numCounters)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
