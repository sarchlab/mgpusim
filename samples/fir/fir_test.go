package main_test

import (
	"testing"

	"gitlab.com/akita/mgpusim/v3/benchmarks/heteromark/fir"
	"gitlab.com/akita/mgpusim/v3/samples/runner"
)

func BenchmarkFIR(t *testing.B) {
	runner := runner.Runner{}
	runner.Timing = true
	runner.Verify = true
	runner.Parallel = true
	runner.GPUIDs = []int{1}
	runner.Init()

	benchmark := fir.NewBenchmark(runner.Driver())
	benchmark.Length = 4096

	runner.AddBenchmark(benchmark)

	runner.Run()
}
