package main

import (
	"flag"

	"gitlab.com/akita/mgpusim/benchmarks/shoc/spmv"
	"gitlab.com/akita/mgpusim/samples/runner"
)

var Dim = flag.Int("Dim", 64, "The number of rows in the input matrix.")
var numIter = flag.Int("iter", 1, "The number of iterations to run.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := spmv.NewBenchmark(runner.GPUDriver)
	benchmark.NumIteration = int32(*numIter)
	benchmark.Dim = int32(*Dim)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
