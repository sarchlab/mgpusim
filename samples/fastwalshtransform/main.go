package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/amdappsdk/fastwalshtransform"
	"gitlab.com/akita/gcn3/samples/runner"
)

var length = flag.Int("length", 1024, "The length of the array that will be transformed")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := fastwalshtransform.NewBenchmark(runner.GPUDriver)
	benchmark.Length = uint32(*length)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
