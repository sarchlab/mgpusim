package main_test

import (
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/heteromark/fir"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

func BenchmarkFIR(t *testing.B) {
	config := sample.Config{Timing: true, Verify: true, Parallel: true, GPUIDs: []int{1}, GPUType: "r9nano"}
	simulation, runtime, finish := config.Build()
	defer finish()
	r := runner.New(simulation, runtime)
	benchmark := fir.NewBenchmark(runtime)
	benchmark.Length = 4096

	config.Configure(benchmark)
	r.AddBenchmark(benchmark)

	if err := simulation.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
}
