package main

import (
	"flag"

	"gitlab.com/akita/mgpusim/benchmarks/amdappsdk/nbody"
	"gitlab.com/akita/mgpusim/samples/runner"
)

//var numIter = flag.Int("iter", 5, "The number of iterations to run.")
//var dimension = flag.Int("dim", 64, "The number of columns/rows in the matrix.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := nbody.NewBenchmark(runner.GPUDriver)
	//benchmark.NumIteration = int32(*numIter)
	//benchmark.Dim = int32(*dimension)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
