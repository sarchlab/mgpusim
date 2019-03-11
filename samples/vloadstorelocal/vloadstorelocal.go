package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/heteromark/vloadstorelocal"
	"gitlab.com/akita/gcn3/samples/runner"
)

func main() {
	flag.Parse()

	runner := runner.Runner{}
	runner.Init()

	benchmark := vloadstorelocal.NewBenchmark(runner.GPUDriver)
	runner.AddBenchmark(benchmark)

	runner.Run()
}
