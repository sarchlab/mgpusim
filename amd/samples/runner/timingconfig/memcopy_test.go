package timingconfig_test

import (
	"slices"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig"
)

func TestMI300XDeviceCopy(t *testing.T) {
	s := sim.MakeBuilder().WithTaskTracking().Build()
	r := timingconfig.MakeBuilder().WithSimulation(s).WithNumGPUs(1).WithGPUType("mi300x").Build()
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Terminate() })
	ctx := r.Init()
	q := r.CreateCommandQueue(ctx)
	// Nonzero, distinct values ensure a misaddressed copy cannot silently pass.
	input := make([]uint32, 256)
	for i := range input {
		input[i] = uint32(i)*17 + 3
	}
	src := r.AllocateMemory(ctx, uint64(len(input)*4))
	dst := r.AllocateMemory(ctx, uint64(len(input)*4))
	benchmarks.Upload(r, q, src, input)
	r.EnqueueMemCopyD2D(q, dst, src, len(input)*4)
	output := benchmarks.Download(r, q, dst, len(input)*4)
	for _, comp := range s.Components() {
		if d, ok := comp.(*driver.Comp); ok {
			d.TickLater()
		}
	}
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	for _, n := range r.Unfinished() {
		if n != 0 {
			t.Fatal("copy queue did not drain")
		}
	}
	got := make([]uint32, len(input))
	if err := benchmarks.Read(r, output, got); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, input) {
		t.Fatalf("device copy returned %v, want %v", got, input)
	}
}
