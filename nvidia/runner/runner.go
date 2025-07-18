package runner

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/simulation"
	"github.com/sarchlab/akita/v4/tracing"
	"github.com/sarchlab/mgpusim/v4/nvidia/benchmark"
	"github.com/sarchlab/mgpusim/v4/nvidia/driver"
	"github.com/sarchlab/mgpusim/v4/nvidia/platform"
)

type Runner struct {
	platform   *platform.Platform
	benchmarks []*benchmark.Benchmark
	simulation *simulation.Simulation
}

func (r *Runner) AddBenchmark(benchmark *benchmark.Benchmark) {
	r.benchmarks = append(r.benchmarks, benchmark)
}

func (r *Runner) Run() {
	// simulationBuilder := simulation.MakeBuilder()
	// r.simulation = simulationBuilder.Build()
	// tracing.StartTask(r.simulation.ID(), "", )

	for _, benchmark := range r.benchmarks {
		execs := benchmark.TraceExecs
		for _, exec := range execs {
			exec.Run(r.Driver())
		}
	}
	r.configureVisTracing()
	r.Driver().LogSimulationStart()
	r.Driver().TickLater()
	r.Engine().Run()
	r.Driver().LogSimulationTerminate()
	r.simulation.Terminate()
	// 	r.Engine().Finished()
}

func (r *Runner) Driver() *driver.Driver {
	return r.platform.Driver
}

func (r *Runner) Engine() sim.Engine {
	return r.platform.Engine
}

func (r *Runner) configureVisTracing() {
	visTracer := r.simulation.GetVisTracer()
	for _, comp := range r.simulation.Components() {
		tracing.CollectTrace(comp.(tracing.NamedHookable), visTracer)
	}
}
