package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/memorybandwidth"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var numElements = flag.Int("size", 4096,
	"The number of float32 elements copied device-to-device.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := memorybandwidth.NewBenchmark(runner.Driver())
	benchmark.NumElements = *numElements
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
