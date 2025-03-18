package main_test

import (
	"testing"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/heteromark/fir"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
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
