package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/amdappsdk/bitonicsort"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var length = flag.Int("length", 1024, "The length of array to sort.")
var orderAscending = flag.Bool("order-asc", true, "Sorting in ascending order.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := bitonicsort.NewBenchmark(runner.Driver())
	benchmark.Length = *length
	benchmark.OrderAscending = *orderAscending

	runner.AddBenchmark(benchmark)

	runner.Run()
}
