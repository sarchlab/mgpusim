package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/dnn/maxpooling"
	"gitlab.com/akita/gcn3/samples/runner"
)

var n = flag.Int("n", 1, "Batch size.")
var c = flag.Int("c", 1, "Channel size.")
var h = flag.Int("h", 32, "Height.")
var w = flag.Int("w", 32, "Weight.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := maxpooling.NewBenchmark(
		runner.GPUDriver,
		*n, *c, *h, *w)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
