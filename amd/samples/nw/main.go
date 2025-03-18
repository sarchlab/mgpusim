package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/nw"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
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
