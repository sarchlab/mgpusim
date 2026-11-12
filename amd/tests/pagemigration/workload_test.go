package pagemigration

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/amdappsdk/matrixmultiplication"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/heteromark/fir"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/polybench/atax"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig"
)

// workload is a benchmark of the samples, run on several GPUs with unified
// memory and verified against its CPU reference.
type workload struct {
	name    string
	gpuType string
	gpus    []int
	policy  string
	slow    bool

	// parallel runs the simulation with the parallel engine.
	parallel bool

	build func(rt *gpuruntime.Runtime, a arch.Type) benchmarks.Benchmark
}

func firWorkload(length int) func(*gpuruntime.Runtime, arch.Type) benchmarks.Benchmark {
	return func(rt *gpuruntime.Runtime, a arch.Type) benchmarks.Benchmark {
		b := fir.NewBenchmark(rt)
		b.Length = length
		b.Arch = a

		return b
	}
}

func ataxWorkload(n int) func(*gpuruntime.Runtime, arch.Type) benchmarks.Benchmark {
	return func(rt *gpuruntime.Runtime, a arch.Type) benchmarks.Benchmark {
		b := atax.NewBenchmark(rt)
		b.NX, b.NY = n, n
		b.Arch = a

		return b
	}
}

func matrixWorkload(n uint32) func(*gpuruntime.Runtime, arch.Type) benchmarks.Benchmark {
	return func(rt *gpuruntime.Runtime, a arch.Type) benchmarks.Benchmark {
		b := matrixmultiplication.NewBenchmark(rt)
		b.X, b.Y, b.Z = n, n, n
		b.Arch = a

		return b
	}
}

// The multi-GPU unified-memory workloads of the samples, with both policies
// on both platforms. FIR gives each GPU a part of the same buffers; ATAX
// runs on the last GPU over memory that starts on the first. (FIR does not
// split its work between MI300X GPUs, with or without page migration, so
// ATAX is the MI300X workload.) The two slow ones start a kernel on hundreds
// of cold pages of another GPU at once, and let four GPUs fault on pages
// that are migrating; faults that were dropped or ignored there left their
// kernels waiting forever. They run only with -slow.
var workloads = []workload{
	{name: "fir", gpuType: "r9nano", gpus: []int{1, 2}, build: firWorkload(8192)},
	{name: "fir", gpuType: "r9nano", gpus: []int{1, 2, 3, 4}, build: firWorkload(8192)},
	{name: "fir", gpuType: "r9nano", gpus: []int{1, 2},
		policy: driver.PageMigrationCounter, build: firWorkload(8192)},
	{name: "fir-parallel-engine", gpuType: "r9nano", gpus: []int{1, 2},
		parallel: true, build: firWorkload(8192)},
	{name: "atax", gpuType: "r9nano", gpus: []int{1, 2}, build: ataxWorkload(256)},
	{name: "atax", gpuType: "mi300x", gpus: []int{1, 2}, build: ataxWorkload(256)},
	{name: "atax", gpuType: "mi300x", gpus: []int{1, 2},
		policy: driver.PageMigrationCounter, build: ataxWorkload(256)},
	{name: "atax-1024", gpuType: "r9nano", gpus: []int{1, 2}, slow: true, build: ataxWorkload(1024)},
	{name: "matrixmultiplication-512", gpuType: "r9nano", gpus: []int{1, 2, 3, 4},
		slow: true, build: matrixWorkload(512)},
}

func TestSampleWorkloadsOnSeveralGPUs(t *testing.T) {
	if testing.Short() {
		t.Skip("page-migration acceptance builds and runs full simulations; run without -short")
	}

	for _, w := range workloads {
		policy := w.policy
		if policy == "" {
			policy = driver.PageMigrationFault
		}

		name := w.gpuType + "/" + w.name + "/" + policy + "/" + gpuCount(w.gpus)
		t.Run(name, func(t *testing.T) {
			if w.slow && !*slowWorkloads {
				t.Skip("pass -slow to run the slow workloads")
			}

			st := runWorkload(t, w, policy)
			if st.PagesMigrated == 0 {
				t.Fatalf("no page migrated: %+v", st)
			}

			t.Logf("%+v", st)
		})
	}
}

// runWorkload runs a workload to its end. The benchmark verifies its own
// output, and the runner reports a queue that stalls.
func runWorkload(t *testing.T, w workload, policy string) driver.PageMigrationStats {
	t.Helper()

	spec := driver.DefaultPageMigrationSpec()
	spec.Policy = policy

	// The counters of a short run must reach their threshold.
	if policy == driver.PageMigrationCounter {
		spec.AccessCounterThreshold = 16
	}

	simBuilder := sim.MakeBuilder()
	if w.parallel {
		simBuilder = simBuilder.WithParallelEngine()
	}

	s := simBuilder.Build()
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})

	rt := timingconfig.MakeBuilder().
		WithSimulation(s).
		WithNumGPUs(len(w.gpus)).
		WithGPUType(w.gpuType).
		WithPageMigration(spec).
		Build()

	a := arch.GCN3
	if w.gpuType == "mi300x" {
		a = arch.CDNA3
	}

	b := w.build(rt, a)
	b.SelectGPU(w.gpus)
	b.SetUnifiedMemory()
	b.SetVerification(true)

	r := runner.New(s, rt)
	r.AddBenchmark(b)

	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}

	if err := r.Run(); err != nil {
		t.Fatal(err)
	}

	for _, c := range s.Components() {
		if d, ok := c.(*driver.Comp); ok {
			return d.State.PageMigration.Stats
		}
	}

	t.Fatal("no driver")

	return driver.PageMigrationStats{}
}
