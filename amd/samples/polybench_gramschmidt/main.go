package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/polybench/gramschmidt"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var m = flag.Int("m", 32, "The number of rows M of matrix A (M x N).")
var n = flag.Int("n", 32, "The number of columns N of matrix A (M x N).")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := gramschmidt.NewBenchmark(runner.Driver())
	benchmark.M = *m
	benchmark.N = *n
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
