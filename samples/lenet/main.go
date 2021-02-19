package main

import (
	"flag"
	"gitlab.com/akita/mgpusim/benchmarks/dnn/lenet"
	"math/rand"

	"gitlab.com/akita/mgpusim/samples/runner"
)

func main() {
	rand.Seed(1)
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := lenet.NewBenchmark(runner.GPUDriver)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
