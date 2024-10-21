package main

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/benchmark"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/platform"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/runner"
	"github.com/tebeka/atexit"
)

func main() {
	benchmark := new(benchmark.BenchmarkBuilder).
		// WithTraceDirectory("data/bfs-rodinia-2.0-ft").
		WithTraceDirectory("data/simple-trace-example").
		Build()

	platform := new(platform.A100PlatformBuilder).
		WithFreq(1 * sim.Hz).
		Build()

	runner := new(runner.RunnerBuilder).
		WithPlatform(platform).
		Build()
	runner.AddBenchmark(benchmark)

	runner.Run()

	atexit.Exit(0)
}
