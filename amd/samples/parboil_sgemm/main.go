// Package main runs the Parboil SGEMM benchmark.
package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/parboil/sgemm"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var nDim = flag.Int("n", 512, "Matrix dimension N (N x N matrix)")

func main() {
	flag.Parse()

	r := new(runner.Runner).Init()

	benchmark := sgemm.NewBenchmark(r.Driver())
	benchmark.N = *nDim

	r.AddBenchmark(benchmark)
	r.Run()
}
