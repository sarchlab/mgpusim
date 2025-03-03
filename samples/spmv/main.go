package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/benchmarks/shoc/spmv"
	"github.com/sarchlab/mgpusim/v4/samples/runner"
)

// Dim is dimension
var Dim = flag.Int("dim", 128, "The number of rows in the input matrix.")

// Sparsity is sparsity
var Sparsity = flag.Float64("sparsity", 0.01,
	"The ratio between non-zero elements to all the elelements in the matrix")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := spmv.NewBenchmark(runner.Driver())
	benchmark.Dim = int32(*Dim)
	benchmark.Sparsity = *Sparsity

	runner.AddBenchmark(benchmark)

	runner.Run()
}
