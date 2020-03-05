package main

import (
	"flag"

	"gitlab.com/akita/mgpusim/benchmarks/shoc/spmv"
	"gitlab.com/akita/mgpusim/samples/runner"
)

var Dim = flag.Int("Dim", 8192, "The number of rows in the input matrix.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := spmv.NewBenchmark(runner.GPUDriver)
	benchmark.Dim = int32(*Dim)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
