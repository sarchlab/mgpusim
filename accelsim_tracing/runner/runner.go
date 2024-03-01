package runner

import (
	"github.com/sarchlab/accelsimtracing/benchmark"
	"github.com/sarchlab/accelsimtracing/platform"
)

type Runner struct {
	platform   *platform.Platform
	benchmarks []*benchmark.Benchmark
}

func (r *Runner) AddBenchmark(benchmark *benchmark.Benchmark) {
	r.benchmarks = append(r.benchmarks, benchmark)
}

func (r *Runner) Run() {
	for _, benchmark := range r.benchmarks {
		execs := benchmark.TraceExecs()
		for _, exec := range execs {
			exec.Run()
		}
	}

	r.platform.Driver().TickLater(r.platform.Engine().CurrentTime())
	r.platform.Engine().Run()

	r.platform.Engine().Finished()
}
