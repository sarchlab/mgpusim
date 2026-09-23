package runner

import (
	"fmt"

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
	// VisTracing records a Daisen visualization trace into the output
	// database.
	VisTracing bool
	// OutputFile is the name of the SQLite database that Akita writes. An
	// empty name lets Akita choose one.
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

// Run simulates the trace in opts.TraceDir on opts.Device.
func Run(opts Options) (Result, error) {
	bm := benchmark.Load(opts.TraceDir)

	builder := simulation.MakeBuilder().WithoutMonitoring()
	if opts.VisTracing {
		builder = builder.WithVisTracingOnStart()
	}

	if opts.OutputFile != "" {
		builder = builder.WithOutputFileName(opts.OutputFile)
	}

	sim := builder.Build()
	defer sim.Terminate()

	p := platform.Build(sim, opts.Device)

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

	if err := sim.GetEngine().Run(); err != nil {
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
