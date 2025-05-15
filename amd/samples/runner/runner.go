// Package runner defines how default benchmark samples are executed.
package runner

import (
	"log"

	// Enable profiling
	_ "net/http/pprof"
	"sync"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/simulation"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner/emusystem"
)

type verificationPreEnablingBenchmark interface {
	benchmarks.Benchmark

	EnableVerification()
}

// Runner is a class that helps running the benchmarks in the official samples.
type Runner struct {
	simulation *simulation.Simulation
	platform   *sim.Domain

	Timing                     bool
	Verify                     bool
	Parallel                   bool
	ReportInstCount            bool
	ReportCacheLatency         bool
	ReportCacheHitRate         bool
	ReportTLBHitRate           bool
	ReportRDMATransactionCount bool
	ReportDRAMTransactionCount bool
	UseUnifiedMemory           bool
	ReportSIMDBusyTime         bool
	ReportCPIStack             bool

	GPUIDs     []int
	benchmarks []benchmarks.Benchmark
}

// Init initializes the platform simulate
func (r *Runner) Init() *Runner {
	r.parseFlag()

	log.SetFlags(log.Llongfile | log.Ldate | log.Ltime)

	r.initSimulation()

	if r.Timing {
		r.buildTimingPlatform()
	} else {
		r.buildEmuPlatform()
	}

	r.createUnifiedGPUs()

	return r
}

func (r *Runner) initSimulation() {
	builder := simulation.MakeBuilder()

	if *parallelFlag {
		builder = builder.WithParallelEngine()
	}

	r.simulation = builder.Build()
}

func (r *Runner) buildEmuPlatform() {
	b := emusystem.MakeBuilder().
		WithSimulation(r.simulation).
		WithNumGPUs(r.GPUIDs[len(r.GPUIDs)-1])

	if *isaDebug {
		b = b.WithDebugISA()
	}

	r.platform = b.Build()
}

func (r *Runner) buildTimingPlatform() {

}

func (r *Runner) createUnifiedGPUs() {
	if *unifiedGPUFlag == "" {
		return
	}

	driver := r.simulation.GetComponentByName("Driver").(*driver.Driver)
	unifiedGPUID := driver.CreateUnifiedGPU(nil, r.GPUIDs)
	r.GPUIDs = []int{unifiedGPUID}
}

// AddBenchmark adds an benchmark that the driver runs
func (r *Runner) AddBenchmark(b benchmarks.Benchmark) {
	b.SelectGPU(r.GPUIDs)
	if r.UseUnifiedMemory {
		b.SetUnifiedMemory()
	}

	r.benchmarks = append(r.benchmarks, b)
}

// AddBenchmarkWithoutSettingGPUsToUse allows for user specified GPUs for
// the benchmark to run.
func (r *Runner) AddBenchmarkWithoutSettingGPUsToUse(b benchmarks.Benchmark) {
	if r.UseUnifiedMemory {
		b.SetUnifiedMemory()
	}
	r.benchmarks = append(r.benchmarks, b)
}

// Run runs the benchmark
func (r *Runner) Run() {
	r.Driver().Run()

	var wg sync.WaitGroup
	for _, b := range r.benchmarks {
		wg.Add(1)
		go func(b benchmarks.Benchmark, wg *sync.WaitGroup) {
			if r.Verify {
				if b, ok := b.(verificationPreEnablingBenchmark); ok {
					b.EnableVerification()
				}
			}

			b.Run()

			if r.Verify {
				b.Verify()
			}
			wg.Done()
		}(b, &wg)
	}
	wg.Wait()

	r.Driver().Terminate()
	r.simulation.Terminate()
}

// Driver returns the GPU driver used by the current runner.
func (r *Runner) Driver() *driver.Driver {
	return r.simulation.GetComponentByName("Driver").(*driver.Driver)
}

// Engine returns the event-driven simulation engine used by the current runner.
func (r *Runner) Engine() sim.Engine {
	return r.simulation.GetEngine()
}
