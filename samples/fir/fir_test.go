package main_test

import (
	"testing"

	"gitlab.com/akita/mgpusim/benchmarks/heteromark/fir"
	"gitlab.com/akita/mgpusim/samples/runner"
)

func BenchmarkFIR(t *testing.B) {
	runner := runner.Runner{}
	runner.Timing = true
	runner.Verify = true
	runner.Parallel = true
	runner.GPUIDs = []int{1}
	runner.Init()

	benchmark := fir.NewBenchmark(runner.GPUDriver)
	benchmark.Length = 4096

	runner.AddBenchmark(benchmark)

	runner.Run()
}
