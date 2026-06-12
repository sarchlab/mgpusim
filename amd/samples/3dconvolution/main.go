package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/polybench/3dconvolution"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var niFlag = flag.Int("ni", 64, "The size of the i dimension.")
var njFlag = flag.Int("nj", 64, "The size of the j dimension.")
var nkFlag = flag.Int("nk", 64, "The size of the k dimension.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := threeconv.NewBenchmark(runner.Driver())
	benchmark.NI = *niFlag
	benchmark.NJ = *njFlag
	benchmark.NK = *nkFlag

	runner.AddBenchmark(benchmark)

	runner.Run()
}
