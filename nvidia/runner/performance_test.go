package runner_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/nvidia/benchmark"
	"github.com/sarchlab/mgpusim/v5/nvidia/platform"
)

// BenchmarkNVIDIAExecution excludes construction and measures repeated kernel
// execution, including scheduling, index maintenance, and memory traffic.
func BenchmarkNVIDIAExecution(b *testing.B) {
	bm := benchmark.Load("../trace/testdata/vectoradd")
	d := platform.H100()
	d.Freq = timing.GHz
	d.GPU.NumSMs = 2
	d.GPU.NumSMSPsPerSM = 2
	d.GPU.NumMemoryBanks = 2
	d.GPU.L2CacheSize = 32768
	d.LaunchOverheadLatency = 5
	b.ReportAllocs()
	for range b.N {
		b.StopTimer()
		s := sim.MakeBuilder().Build()
		p := platform.Build(s, d)
		if err := s.Initialize(); err != nil {
			b.Fatal(err)
		}
		for range 128 {
			for _, w := range bm.TraceExecs {
				w.Run(p.Driver)
			}
		}
		p.Driver.TickLater()
		b.StartTimer()
		if err := s.Engine().Run(); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		if p.Driver.UnfinishedKernels() != 0 {
			b.Fatal("unfinished workload")
		}
		b.Logf("finish=%d ps", p.Driver.FinishTime())
		if err := s.Terminate(); err != nil {
			b.Fatal(err)
		}
	}
}
