// Package sample composes the hardware and reporting used by command-line examples.
package sample

import (
	"log"
	_ "net/http/pprof"
	"os"

	"github.com/rs/xid"
	"github.com/sarchlab/akita/v5/monitoring"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig"
)

// Config holds only command-line choices; it owns no hardware or workload.
type Config struct {
	Timing, Verify, Parallel, UseUnifiedMemory bool
	ArchType                                   arch.Type
	GPUType                                    string
	GPUIDs                                     []int
}

func ParseConfig() Config { var c Config; c.parseFlag(); return c }

type assembly struct {
	Config
	simulation *simulation.Simulation
	gpuDriver  *gpuruntime.Runtime
	reporter   *reporter
	monitor    *monitoring.Monitor
}

// Build registers the chosen platform. The returned finish function flushes
// reports and terminates the simulation after the caller's Runner completes.
func (c *Config) Build() (*simulation.Simulation, *gpuruntime.Runtime, func() error) {
	log.SetFlags(log.Llongfile | log.Ldate | log.Ltime)
	a := assembly{Config: *c}
	a.initSimulation()
	if c.Timing {
		a.buildTimingPlatform()
	} else {
		a.buildEmuPlatform()
	}
	a.createUnifiedGPUs()
	c.GPUIDs = a.GPUIDs
	s, r, report := a.simulation, a.gpuDriver, a.reporter
	return s, r, func() error { return finishSimulation(s, report) }
}

// Configure applies software workload choices without constructing hardware.
func (c Config) Configure(b benchmarks.Benchmark) {
	b.SelectGPU(c.GPUIDs)
	if c.UseUnifiedMemory {
		b.SetUnifiedMemory()
	}
	b.SetVerification(c.Verify)
}
func (r *assembly) initSimulation() {
	builder := simulation.MakeBuilder().WithTaskTracking()

	if r.Parallel {
		builder = builder.WithParallelEngine()
	}

	if !*disableAkitaRTM {
		mb := monitoring.MakeBuilder()
		if *customPortForAkitaRTM > 0 {
			mb = mb.WithPortNumber(*customPortForAkitaRTM)
		}
		r.monitor = mb.Build()
		builder = builder.WithMonitor(r.monitor)
	}

	if *visTracing {
		builder = builder.WithVisTracingOnStart()
		builder = withRecordedSource(builder)
	}

	// Only honor -metric-file-name when it is explicitly set, so the
	// default output name stays the v4-compatible "akita_sim_<id>".
	output := "akita_sim_" + xid.New().String()
	explicitOutput := metricFileNameFlagIsSet() && *filenameFlag != ""
	if explicitOutput {
		output = *filenameFlag
	}
	// beta.10 always appends this suffix to the supplied base name.
	output += ".sqlite3"
	// Preserve the sample CLI's replacement behavior for an explicit filename.
	// Akita rc1 itself only creates recordings at previously unused paths.
	if explicitOutput {
		if err := os.Remove(output); err != nil && !os.IsNotExist(err) {
			panic(err)
		}
	}
	builder = builder.WithOutputPath(output)

	r.simulation = builder.Build()
}

func (r *assembly) buildEmuPlatform() {
	b := emusystem.MakeBuilder().
		WithSimulation(r.simulation).
		WithNumGPUs(r.GPUIDs[len(r.GPUIDs)-1]).
		WithArchitecture(r.ArchType)

	if *isaDebug {
		b = b.WithDebugISA()
	}

	r.gpuDriver = b.Build()
}

func (r *assembly) buildTimingPlatform() {
	b := timingconfig.MakeBuilder().WithMonitor(r.monitor).
		WithSimulation(r.simulation).
		WithNumGPUs(r.GPUIDs[len(r.GPUIDs)-1]).
		WithGPUType(r.GPUType)

	if *magicMemoryCopy {
		b = b.WithMagicMemoryCopy()
	}

	r.gpuDriver = b.Build()
	r.reporter = newReporter(r.simulation)
}

func (r *assembly) createUnifiedGPUs() {
	if *unifiedGPUFlag == "" {
		return
	}

	unifiedGPUID := r.gpuDriver.CreateUnifiedGPU(0, r.GPUIDs)
	r.GPUIDs = []int{unifiedGPUID}
}

func (c Config) ConfigureWithoutGPUs(b benchmarks.Benchmark) {
	if c.UseUnifiedMemory {
		b.SetUnifiedMemory()
	}
	b.SetVerification(c.Verify)
}

func finishSimulation(s *simulation.Simulation, report *reporter) error {
	if report != nil {
		report.report()
		report.dataRecorder.Flush()
	}
	return s.Terminate()
}
