package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/md5hash"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numElements = flag.Int("num_elements", 1048576, "Number of 64-byte blocks to hash")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := md5hash.NewBenchmark(runner.Driver())
	benchmark.SetNumElements(*numElements)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
