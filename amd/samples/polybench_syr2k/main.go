package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/polybench/syr2k"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var n = flag.Int("size", 64, "The matrix dimension N (C is N×N, A and B are N×M).")
var m = flag.Int("inner-size", 64, "The contraction dimension M (A and B are N×M).")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := syr2k.NewBenchmark(runner.Driver())
	benchmark.N = *n
	benchmark.M = *m
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
