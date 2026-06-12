package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/shoc/gemm"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var mDim = flag.Int("m", 512, "Number of rows in A and C")
var nDim = flag.Int("n", 512, "Number of columns in B and C")
var kDim = flag.Int("k", 512, "Inner dimension (columns of A, rows of B)")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := gemm.NewBenchmark(runner.Driver())
	benchmark.M = *mDim
	benchmark.N = *nDim
	benchmark.K = *kDim

	runner.AddBenchmark(benchmark)

	runner.Run()
}
