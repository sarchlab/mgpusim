package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/layernorm"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var rows = flag.Int("rows", 1024, "Number of rows")
var hid = flag.Int("hid", 4096, "Hidden dimension size")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := layernorm.NewBenchmark(runner.Driver())
	benchmark.SetRows(*rows)
	benchmark.SetHid(*hid)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
