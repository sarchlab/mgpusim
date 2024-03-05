package runner

import (
	"github.com/sarchlab/accelsimtracing/benchmark"
	"github.com/sarchlab/accelsimtracing/driver"
	"github.com/sarchlab/accelsimtracing/platform"
	"github.com/sarchlab/akita/v3/sim"
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
		execs := benchmark.TraceExecs
		for _, exec := range execs {
			exec.Run(r.Driver())
		}
	}

	r.Driver().TickLater(r.Engine().CurrentTime())
	r.Engine().Run()

	r.Engine().Finished()
}

func (r *Runner) Driver() *driver.Driver {
	return r.platform.Driver
}

func (r *Runner) Engine() sim.Engine {
	return r.platform.Engine
}
