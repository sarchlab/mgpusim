package runner

import (
	"fmt"
	"os"
	"time"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"

	"github.com/sarchlab/mgpusim/v5/nvidia/benchmark"
	"github.com/sarchlab/mgpusim/v5/nvidia/platform"
)

// Options configures a simulation run.
type Options struct {
	// TraceDir is the directory that contains kernelslist.g.
	TraceDir string
	// Device is the GPU model to simulate.
	Device platform.Device
	// VisTracing records a Daisen visualization trace into an SQLite
	// database. Tracing makes the simulation much slower.
	VisTracing bool
	// Progress, if not zero, prints the simulation status to stderr about
	// this often (in wall-clock time).
	Progress time.Duration
	// OutputFile is the name of that database. An empty name lets Akita
	// choose one. It is only used with VisTracing.
	OutputFile string
}

// Result summarizes a simulation run.
type Result struct {
	Device     string
	Freq       timing.Freq
	NumKernels int
	NumWarps   uint64
	NumInsts   uint64
	// Time is the simulated time when the last kernel finished.
	Time timing.VTimeInPicoSec
}

// Cycles returns the simulated execution time in core clock cycles.
func (r Result) Cycles() uint64 {
	return r.Freq.Cycle(r.Time)
}

// Seconds returns the simulated execution time in seconds.
func (r Result) Seconds() float64 {
	return float64(r.Time) * 1e-12
}

// progressStep is the simulated time between two progress checks.
const progressStep = 10 * 1000 * 1000 // 10 us in ps

// runEngine runs the simulation to the end. With progress reporting, it runs
// the serial engine in slices of simulated time and prints the status after
// a slice once the reporting interval has passed.
func runEngine(
	engine timing.Engine,
	p *platform.Platform,
	interval time.Duration,
) error {
	serial, ok := engine.(*timing.SerialEngine)
	if interval <= 0 || !ok {
		return engine.Run()
	}

	start := time.Now()
	lastReport := start

	for p.Driver.UnfinishedKernels() > 0 {
		now := serial.CurrentTime()
		if err := serial.RunUntil(now + progressStep); err != nil {
			return err
		}

		if time.Since(lastReport) >= interval {
			lastReport = time.Now()
			printStatus(p, serial.CurrentTime(), time.Since(start))
		}

		if serial.CurrentTime() == now && p.Driver.UnfinishedKernels() > 0 {
			printStatus(p, serial.CurrentTime(), time.Since(start))

			return fmt.Errorf("no more events but %d kernel(s) did not finish",
				p.Driver.UnfinishedKernels())
		}
	}

	return engine.Run()
}

func printStatus(p *platform.Platform, now timing.VTimeInPicoSec, wall time.Duration) {
	var insts uint64

	var warps, unsent, inflight int

	for _, g := range p.Devices {
		for _, s := range g.SMList {
			for _, sp := range s.SMSPs {
				insts += sp.GetTotalInstsCount()
				w, u, i := sp.Status()
				warps += w
				unsent += u
				inflight += i
			}
		}
	}

	undispatched, unfinished := p.Devices[0].Status()

	fmt.Fprintf(os.Stderr,
		"[%6.1fs] sim %9.3f us | kernels left %d | TBs waiting %d, unfinished %d"+
			" | warps %d | insts %d | mem unsent %d, in flight %d\n",
		wall.Seconds(), float64(now)*1e-6, p.Driver.UnfinishedKernels(),
		undispatched, unfinished, warps, insts, unsent, inflight)
}

// newSimulation creates the engine and the registrar that components are
// built with.
//
// Components registered with an Akita simulation get a tracing hook. The
// hook keeps every milestone of a task in memory and scans them on each new
// one, so a request that stalls for many cycles makes the simulation
// quadratically slow even when no trace is recorded. Without -trace-vis the
// components are therefore built with a standalone registrar on a bare
// engine, which attaches no hooks and writes no database.
func newSimulation(opts Options) (timing.Engine, modeling.Registrar, func()) {
	if !opts.VisTracing {
		engine := timing.NewSerialEngine()
		return engine, modeling.NewStandaloneRegistrar(engine), func() {}
	}

	builder := simulation.MakeBuilder().
		WithoutMonitoring().
		WithVisTracingOnStart()
	if opts.OutputFile != "" {
		builder = builder.WithOutputFileName(opts.OutputFile)
	}

	sim := builder.Build()

	return sim.GetEngine(), sim, sim.Terminate
}

// Run simulates the trace in opts.TraceDir on opts.Device.
func Run(opts Options) (Result, error) {
	bm := benchmark.Load(opts.TraceDir)

	engine, registrar, done := newSimulation(opts)
	defer done()

	p := platform.Build(registrar, opts.Device)

	result := Result{Device: opts.Device.Name, Freq: opts.Device.Freq}

	for _, exec := range bm.TraceExecs {
		if k, ok := exec.(*benchmark.ExecKernel); ok {
			result.NumKernels++
			for _, tb := range k.Kernel.Threadblocks {
				result.NumWarps += tb.WarpsCount()
			}
		}

		exec.Run(p.Driver)
	}

	p.Driver.TickLater()

	if err := runEngine(engine, p, opts.Progress); err != nil {
		return result, fmt.Errorf("simulation failed: %w", err)
	}

	result.Time = p.Driver.FinishTime()

	for _, g := range p.Devices {
		for _, s := range g.SMList {
			for _, sp := range s.SMSPs {
				result.NumInsts += sp.GetTotalInstsCount()
			}
		}
	}

	return result, nil
}
