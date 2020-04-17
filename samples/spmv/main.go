package main

import (
	"flag"

	"gitlab.com/akita/mgpusim/benchmarks/shoc/spmv"
	"gitlab.com/akita/mgpusim/samples/runner"
)

var Dim = flag.Int("dim", 128, "The number of rows in the input matrix.")
var Sparsity = flag.Float64("sparsity", 0.01,
	"The ratio between non-zero elements to all the elelements in the matrix")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := spmv.NewBenchmark(runner.GPUDriver)
	benchmark.Dim = int32(*Dim)
	benchmark.Sparsity = *Sparsity

	runner.AddBenchmark(benchmark)

	runner.Run()
}
