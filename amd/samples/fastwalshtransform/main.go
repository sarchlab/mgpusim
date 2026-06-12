package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/amdappsdk/fastwalshtransform"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var length = flag.Int("length", 1024, "The length of the array that will be transformed")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := fastwalshtransform.NewBenchmark(runner.Driver())
	benchmark.Length = uint32(*length)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
