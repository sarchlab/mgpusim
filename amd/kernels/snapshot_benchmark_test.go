package kernels

import (
	"github.com/sarchlab/akita/v5/sim"
	"testing"
)

// BenchmarkWorkGroupSnapshotAndRestore isolates dispatch serialization from
// executable size. The 1MiB code remains in canonical GPU storage at execution.
func BenchmarkWorkGroupSnapshotAndRestore(b *testing.B) {
	launch := testLaunch()
	launch.CodeObject.Data = make([]byte, 1<<20)
	grid := NewGridBuilder(sim.MakeBuilder().Build())
	grid.SetKernel(launch)
	group := grid.NextWG()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := WorkGroupSnapshot(group).Restore(); err != nil {
			b.Fatal(err)
		}
	}
}
