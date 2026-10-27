package runner_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/heteromark/fir"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig"
)

// BenchmarkFIRExecution includes host submission, GPU execution and readback.
// Platform construction is excluded; every iteration uses a fresh simulation.
func BenchmarkFIRExecution(b *testing.B) {
	for _, mode := range []string{"GCN3Timing", "GCN3Emulation", "CDNA3Emulation"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				b.StopTimer()
				s := sim.MakeBuilder().Build()
				var rt *gpuruntime.Runtime
				architecture := arch.GCN3
				if mode == "GCN3Timing" {
					rt = timingconfig.MakeBuilder().WithSimulation(s).WithNumGPUs(1).WithGPUType("r9nano").Build()
				} else {
					if mode == "CDNA3Emulation" {
						architecture = arch.CDNA3
					}
					rt = emusystem.MakeBuilder().WithSimulation(s).WithNumGPUs(1).WithArchitecture(architecture).Build()
				}
				work := fir.NewBenchmark(rt)
				work.Length = 8192
				work.Arch = architecture
				work.SelectGPU([]int{1})
				work.SetVerification(true)
				r := runner.New(s, rt)
				r.AddBenchmark(work)
				if err := s.Initialize(); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				if err := r.Run(); err != nil {
					b.Fatal(err)
				}
				b.StopTimer()
				b.Logf("finish=%d ps", s.Engine().CurrentTime())
				if err := s.Terminate(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
