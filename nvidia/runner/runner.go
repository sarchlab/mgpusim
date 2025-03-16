package runner

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/benchmark"
	"github.com/sarchlab/mgpusim/v4/nvidia/driver"
	"github.com/sarchlab/mgpusim/v4/nvidia/platform"
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

	r.Driver().TickLater()
	r.Engine().Run()
	// 	r.Engine().Finished()
}

func (r *Runner) Driver() *driver.Driver {
	return r.platform.Driver
}

func (r *Runner) Engine() sim.Engine {
	return r.platform.Engine
}
