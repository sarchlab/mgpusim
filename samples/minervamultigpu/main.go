package main

import (
	"flag"

	"gitlab.com/akita/mgpusim/benchmarks/dnn/minervamultigpu"
	"gitlab.com/akita/mgpusim/samples/runner"
)

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := minervamultigpu.NewBenchmark(runner.GPUDriver)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
