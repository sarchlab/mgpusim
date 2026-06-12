package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/parboil/spmv_parboil"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numRows = flag.Int("num_rows", 65536, "Number of rows in the sparse matrix")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := spmv_parboil.NewBenchmark(runner.Driver())
	benchmark.NumRows = *numRows

	runner.AddBenchmark(benchmark)

	runner.Run()
}
