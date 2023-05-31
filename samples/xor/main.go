package main

import (
	"flag"
	"math/rand"

	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/xor"
	"github.com/sarchlab/mgpusim/v3/samples/runner"
)

func main() {
	rand.Seed(1)

	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := xor.NewBenchmark(runner.Driver())

	runner.AddBenchmark(benchmark)

	runner.Run()
}
