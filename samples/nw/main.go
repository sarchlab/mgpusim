package main

import (
	"flag"

	"gitlab.com/akita/mgpusim/benchmarks/rodinia/nw"
	"gitlab.com/akita/mgpusim/samples/runner"
)

var length = flag.Int("length", 64, "The number bases in the gene sequence")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := nw.NewBenchmark(runner.GPUDriver)
	benchmark.SetLength(*length)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
