package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/polybench/syr2k"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var n = flag.Int("size", 64, "The matrix dimension N (C is N×N, A and B are N×M).")
var m = flag.Int("inner-size", 64, "The contraction dimension M (A and B are N×M).")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := syr2k.NewBenchmark(runtime)
	benchmark.N = *n
	benchmark.M = *m
	benchmark.Arch = config.ArchType

	config.Configure(benchmark)
	runner.AddBenchmark(benchmark)

	if err := simulation.Initialize(); err != nil {
		panic(err)
	}
	if err := runner.Run(); err != nil {
		panic(err)
	}
	if err := finish(); err != nil {
		panic(err)
	}
}
