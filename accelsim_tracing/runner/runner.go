package runner

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/benchmark"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/driver"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/platform"
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

    // v3
    // r.Driver().TickLater(r.Engine().CurrentTime())
	r.Driver().TickLater()
	r.Engine().Run()
    // v3
    // TODO: r.Engine().Finished()
    // 	r.Engine().Finished()
}

func (r *Runner) Driver() *driver.Driver {
	return r.platform.Driver
}

func (r *Runner) Engine() sim.Engine {
	return r.platform.Engine
}
