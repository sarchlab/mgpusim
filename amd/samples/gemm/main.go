package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/polybench/gemm"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var niFlag = flag.Int("ni", 256, "The number of rows of A and C.")
var njFlag = flag.Int("nj", 256, "The number of columns of B and C.")
var nkFlag = flag.Int("nk", 256, "The number of columns of A / rows of B.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := gemm.NewBenchmark(runner.Driver())
	benchmark.NI = *niFlag
	benchmark.NJ = *njFlag
	benchmark.NK = *nkFlag

	runner.AddBenchmark(benchmark)

	runner.Run()
}
