package benchmarks_test

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/sharedmembandwidth"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/sharedmemlatency"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

type sharedCopyFixture struct {
	benchmark benchmarks.Benchmark
	queue     gpuruntime.CommandQueue
	sink      gpuruntime.Ptr
	want      []float32
	output    func() []float32
}

func sharedCopyBenchmark(rt *gpuruntime.Runtime, name, pattern string, verify bool) sharedCopyFixture {
	if name == "bandwidth" {
		b := sharedmembandwidth.NewBenchmark(rt)
		b.NumBlocks, b.InnerIters, b.BlockSize = 2, 1, 64
		b.Arch, b.AccessPattern, b.Phase = arch.CDNA3, pattern, 1
		b.SelectGPU([]int{1})
		b.SetVerification(verify)
		b.Queue = rt.CreateCommandQueue(b.Context)
		b.GSink = rt.AllocateMemory(b.Context, 8)
		return sharedCopyFixture{b, b.Queue, b.GSink, []float32{8, 8}, func() []float32 { return b.NoConflictResult }}
	}
	b := sharedmemlatency.NewBenchmark(rt)
	b.NumBlocks, b.InnerIters, b.BlockSize = 2, 1, 64
	b.Arch, b.AccessPattern, b.Phase = arch.CDNA3, pattern, 1
	b.SelectGPU([]int{1})
	b.SetVerification(verify)
	b.Queue = rt.CreateCommandQueue(b.Context)
	b.GSink = rt.AllocateMemory(b.Context, 8)
	// One iteration visits 12288/64 cells, each initially 1.0.
	return sharedCopyFixture{b, b.Queue, b.GSink, []float32{192, 192}, func() []float32 { return b.NoConflictResult }}
}

// The gfx942 kernels have a separately recorded baseline VOP3A513 decode gap.
// Seed their CPU-reference sink at the post-kernel phase, then execute real
// simulator H2D/D2H and the workload's actual readback Steps. This tests normal
// output preservation without claiming execution of those unsupported kernels.
func TestSharedMemoryOutputCopiesDoNotDependOnVerification(t *testing.T) {
	for _, name := range []string{"bandwidth", "latency"} {
		for _, pattern := range []string{"no_conflict", "both", "conflict"} {
			for _, verify := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/verify=%v", name, pattern, verify), func(t *testing.T) {
					runSharedCopyCase(t, name, pattern, verify)
				})
			}
		}
	}
}
func runSharedCopyCase(t *testing.T, name, pattern string, verify bool) {
	t.Helper()
	s := sim.MakeBuilder().WithTaskTracking().Build()
	rt := emusystem.MakeBuilder().WithSimulation(s).WithNumGPUs(1).WithArchitecture(arch.CDNA3).Build()
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	}()
	copies := &outputCopies{}
	var ticker driver.Ticker
	for _, component := range s.Components() {
		if component.Name() == "Driver" {
			ticker = component.(driver.Ticker)
			s.TaskTracker().Subscribe(copies, component)
		}
	}
	f := sharedCopyBenchmark(rt, name, pattern, verify)
	upload := benchmarks.Upload(rt, f.queue, f.sink, f.want)
	ticker.TickLater()
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if err := rt.FreeMemory(upload.Context, upload.Address); err != nil {
		t.Fatal(err)
	}
	// Rebind kernels at the saved post-kernel boundary, without submitting work.
	var state bytes.Buffer
	if err := f.benchmark.SaveCheckpoint(&state); err != nil {
		t.Fatal(err)
	}
	if err := f.benchmark.LoadCheckpoint(&state); err != nil {
		t.Fatal(err)
	}
	if err := f.benchmark.Step(); err != nil {
		t.Fatal(err)
	}
	ticker.TickLater()
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if err := f.benchmark.Step(); err != nil {
		t.Fatal(err)
	}
	checkSharedCopyResult(t, f, pattern, copies.count)
	if pattern == "no_conflict" {
		// This final host phase performs CPU comparison only when enabled.
		if err := f.benchmark.Step(); err != nil {
			t.Fatal(err)
		}
		if !f.benchmark.IsEnded() {
			t.Fatal("readback path did not finish")
		}
	} else if rt.Unfinished()[0] == 0 {
		t.Fatal("conflict kernel was not submitted after the copy phase")
	}
}
func checkSharedCopyResult(t *testing.T, f sharedCopyFixture, pattern string, count int) {
	t.Helper()
	if pattern == "conflict" {
		if count != 0 || f.output() != nil {
			t.Fatal("conflict-only path copied nonexistent normal output")
		}
		return
	}
	if count != 1 || !reflect.DeepEqual(f.output(), f.want) {
		t.Fatalf("normal output: copies=%d got=%v want=%v", count, f.output(), f.want)
	}
}
