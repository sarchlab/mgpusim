package main

import (
	"flag"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/benchmark"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/platform"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/runner"
	"github.com/tebeka/atexit"
)

type Params struct {
	TraceDir *string
}

// get trace directory from parameter
func parseFlags() *Params {
	params := &Params{
		TraceDir: flag.String("trace-dir", "data/simple-trace-example", "The directory that contains the trace files"),
	}

	flag.Parse()

	return params
}

func main() {
	params := parseFlags()

	benchmark := new(benchmark.BenchmarkBuilder).
		WithTraceDirectory(*params.TraceDir).
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
