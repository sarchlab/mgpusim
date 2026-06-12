package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/polybench/2dconvolution"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var niFlag = flag.Int("ni", 256, "The number of rows.")
var njFlag = flag.Int("nj", 256, "The number of columns.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := twoconv.NewBenchmark(runner.Driver())
	benchmark.NI = *niFlag
	benchmark.NJ = *njFlag

	runner.AddBenchmark(benchmark)

	runner.Run()
}
