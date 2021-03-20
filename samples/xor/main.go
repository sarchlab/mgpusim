package main

import (
	"flag"
	"math/rand"

	"gitlab.com/akita/mgpusim/v2/benchmarks/dnn/xor"
	"gitlab.com/akita/mgpusim/v2/samples/runner"
)

func main() {
	rand.Seed(1)

	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := xor.NewBenchmark(runner.Driver())

	runner.AddBenchmark(benchmark)

	runner.Run()
}
