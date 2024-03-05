package main

import (
	"github.com/sarchlab/accelsimtracing/benchmark"
	"github.com/sarchlab/accelsimtracing/platform"
	"github.com/sarchlab/accelsimtracing/runner"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/tebeka/atexit"
)

func main() {
	benchmark := new(benchmark.BenchmarkBuilder).
		WithTraceDirectory("data/bfs-rodinia-2.0-ft").
		// WithTraceDirectory("data/simple").
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
