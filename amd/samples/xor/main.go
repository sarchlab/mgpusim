package main

import (
	"flag"
	"math/rand"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training_benchmarks/xor"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

func main() {
	rand.Seed(1)

	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := xor.NewBenchmark(runner.Driver())

	runner.AddBenchmark(benchmark)

	runner.Run()
}
