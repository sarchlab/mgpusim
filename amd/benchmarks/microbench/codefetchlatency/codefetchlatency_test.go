package codefetchlatency

import (
	"bytes"
	"debug/elf"
	"strings"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
)

func TestCheckParameters(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		body, loops, blocks, threads int
		rejected                     string
	}{
		{"default launch", 256, 1, 1, 64, ""},
		{"largest chain", 65536, 16, 1, 64, ""},
		{"largest work-group", 16, 1, 1, 1024, ""},
		{"largest grid", 16, 1, 4194303, 1024, ""},
		{"unsupported body size", 100, 1, 1, 64, "no kernel with 100"},
		{"negative body size", -16, 1, 1, 64, "no kernel with -16"},
		{"no body", 0, 1, 1, 64, "no kernel with 0"},
		{"no loops", 16, 0, 1, 64, "operations per thread"},
		{"no blocks", 16, 1, 0, 64, "1 block or more"},
		{"no threads", 16, 1, 1, 0, "threads per block"},
		{"chain too long", 65536, 17, 1, 64, "operations per thread"},
		{"negative loops", 16, -1, 1, 64, "operations per thread"},
		{"work-group above the gfx942 limit", 16, 1, 1, 1025, "threads per block"},
		{"work-group that narrows to zero", 16, 1, 1, 65536, "threads per block"},
		{"negative work-group", 16, 1, 1, -64, "threads per block"},
		{"negative block count", 16, 1, -1, 64, "1 block or more"},
		{"grid beyond uint32", 16, 1, 4194304, 1024, "1 block or more"},
		{"grid that wraps an int", 16, 1, 1 << 62, 1024, "1 block or more"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckParameters(tc.body, tc.loops, tc.blocks, tc.threads)
			switch {
			case tc.rejected == "" && err != nil:
				t.Fatalf("expected the launch to be accepted, got %v", err)
			case tc.rejected != "" && err == nil:
				t.Fatal("expected the launch to be rejected")
			case tc.rejected != "" && !strings.Contains(err.Error(), tc.rejected):
				t.Fatalf("expected an error about %q, got %v", tc.rejected, err)
			}
		})
	}
}

func TestZeroParametersTakeTheDefaults(t *testing.T) {
	b := &Benchmark{State: &State{}}
	got := [4]int{b.bodyOps(), b.loops(), b.numBlocks(), b.threadsPerBlock()}
	if want := [4]int{256, 1, 1, 64}; got != want {
		t.Fatalf("expected the defaults %v, got %v", want, got)
	}
}

func TestEveryBodySizeHasAKernel(t *testing.T) {
	object, err := elf.NewFile(bytes.NewReader(cdna3HSACOBytes))
	if err != nil {
		t.Fatal(err)
	}
	symbols, err := object.Symbols()
	if err != nil {
		t.Fatal(err)
	}
	kernels := make(map[string]bool)
	for _, symbol := range symbols {
		kernels[symbol.Name] = true
	}
	for _, size := range BodySizes {
		if !kernels[KernelName(size)] {
			t.Errorf("the embedded object has no %s", KernelName(size))
		}
	}
}

// A checkpoint from an executable with other kernels or limits must be
// reported, leaving the benchmark as it was.
func TestAnUnsupportedCheckpointIsReportedAndLeavesTheBenchmarkUnchanged(t *testing.T) {
	saved := &Benchmark{State: &State{BodyOps: 100, Loops: 4}}
	saved.Checkpoint = benchmarks.NewCheckpoint(saved.State, saved.bind)
	var checkpoint bytes.Buffer
	if err := saved.SaveCheckpoint(&checkpoint); err != nil {
		t.Fatal(err)
	}

	b := &Benchmark{State: &State{BodyOps: 64, Loops: 8}}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)
	if err := b.bind(); err != nil {
		t.Fatal(err)
	}
	kernel := b.hsaco

	err := b.LoadCheckpoint(&checkpoint)
	if err == nil || !strings.Contains(err.Error(), "no kernel with 100") {
		t.Fatalf("expected the unsupported body size to be reported, got %v", err)
	}
	if b.BodyOps != 64 || b.Loops != 8 {
		t.Fatalf("expected the parameters to be restored, got %d x %d", b.BodyOps, b.Loops)
	}
	if b.hsaco != kernel {
		t.Fatal("expected the loaded kernel to be kept")
	}
	if b.Step() == nil {
		t.Fatal("expected the next step to report the failed load")
	}
}

func TestACheckpointRestoresTheKernelOfItsBodySize(t *testing.T) {
	saved := &Benchmark{State: &State{BodyOps: 1024, Loops: 4}}
	saved.Checkpoint = benchmarks.NewCheckpoint(saved.State, saved.bind)
	var checkpoint bytes.Buffer
	if err := saved.SaveCheckpoint(&checkpoint); err != nil {
		t.Fatal(err)
	}

	b := &Benchmark{State: &State{BodyOps: 64}}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)
	if err := b.LoadCheckpoint(&checkpoint); err != nil {
		t.Fatal(err)
	}
	if b.BodyOps != 1024 || b.Loops != 4 || b.hsaco == nil {
		t.Fatalf("expected the saved parameters and a kernel, got %d x %d", b.BodyOps, b.Loops)
	}
	if got := len(b.hsaco.Data); got != 8*1024+116 {
		t.Fatalf("expected the 1024-operation kernel of 8308 bytes, got %d bytes", got)
	}
}
