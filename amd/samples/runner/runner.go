// Package runner schedules checkpointed host phases in a stable order.
package runner

import (
	"fmt"

	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// Runner owns only the workload collection and its execution loop.
type Runner struct {
	simulation    *simulation.Simulation
	runtime       *gpuruntime.Runtime
	benchmarks    []benchmarks.Benchmark
	driver        driver.Ticker
	betweenRounds bool
}

func New(s *simulation.Simulation, r *gpuruntime.Runtime) *Runner {
	rnr := &Runner{simulation: s, runtime: r, betweenRounds: true}
	return rnr
}
func (r *Runner) AddBenchmark(b benchmarks.Benchmark) {
	b.SetName(fmt.Sprintf("Benchmark[%d]", len(r.benchmarks)))
	r.benchmarks = append(r.benchmarks, b)
	r.simulation.RegisterResource(b)
}
func (r *Runner) resolveDriver() error {
	for _, c := range r.simulation.Components() {
		if c.Name() != "Driver" {
			continue
		}
		ticker, ok := c.(driver.Ticker)
		if !ok {
			return fmt.Errorf("runner: Driver cannot tick")
		}
		r.driver = ticker
		return nil
	}
	return fmt.Errorf("runner: Driver component missing")
}
func (r *Runner) stalled() error {
	for i, n := range r.runtime.Unfinished() {
		if n != 0 {
			return fmt.Errorf("runner: queue %d stalled with %d unfinished commands", i+1, n)
		}
	}
	return nil
}

// Step runs previously submitted work before visiting each active benchmark.
// False means every workload has completed its final host processing.
func (r *Runner) Step() (bool, error) {
	r.betweenRounds = false
	if r.driver == nil {
		if err := r.resolveDriver(); err != nil {
			return false, err
		}
	}
	if err := r.simulation.Run(); err != nil {
		return false, err
	}
	if err := r.stalled(); err != nil {
		return false, err
	}
	r.runtime.BeginSimulationTrace(r.simulation.TaskTracker(), r.traceOwner())
	active := false
	for _, b := range r.benchmarks {
		if b.IsEnded() {
			continue
		}
		active = true
		if err := b.Step(); err != nil {
			return false, fmt.Errorf("%s: %w", b.Name(), err)
		}
	}
	if active {
		r.driver.TickLater()
	}
	if !active {
		r.runtime.EndSimulationTrace(r.simulation.TaskTracker())
	}
	r.betweenRounds = true
	return active, nil
}
func (r *Runner) Run() error {
	for {
		active, err := r.Step()
		if err != nil {
			return err
		}
		if !active {
			return nil
		}
	}
}

// SaveCheckpoint is permitted only after the complete host round. Callers
// may first use SerialEngine.RunUntil to capture an in-flight event boundary.
func (r *Runner) SaveCheckpoint(path string) error {
	if !r.betweenRounds {
		return fmt.Errorf("runner: checkpoint requested within host round")
	}
	return r.simulation.SaveCheckpoint(path)
}

// traceOwner resolves only the generic task owner, never component execution state.
func (r *Runner) traceOwner() tasks.Owner {
	for _, c := range r.simulation.Components() {
		if c.Name() == "Driver" {
			return c
		}
	}
	return nil
}
