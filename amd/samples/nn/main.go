package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/nn"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numRecords = flag.Int("num_records", 65536, "Number of location records")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := nn.NewBenchmark(runner.Driver())
	benchmark.SetNumRecords(*numRecords)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
