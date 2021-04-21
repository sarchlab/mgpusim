package main

import (
	"flag"

	"gitlab.com/akita/mgpusim/v2/benchmarks/rodinia/nw"
	"gitlab.com/akita/mgpusim/v2/samples/runner"
)

var length = flag.Int("length", 64, "The number bases in the gene sequence")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := nw.NewBenchmark(runner.Driver())
	benchmark.SetLength(*length)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
