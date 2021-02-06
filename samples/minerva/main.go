package main

import (
	"flag"
	"math/rand"

	"gitlab.com/akita/mgpusim/benchmarks/dnn/minerva"
	"gitlab.com/akita/mgpusim/samples/runner"
)

func main() {
	rand.Seed(1)
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := minerva.NewBenchmark(runner.GPUDriver)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
