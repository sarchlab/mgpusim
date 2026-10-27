package smsp

import (
	"testing"

	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

// BenchmarkSchedulerLookup resolves the last of 64 resident warps and the last
// of its 32 in-flight pipelines, exercising both ordered lookups.
func BenchmarkSchedulerLookup(b *testing.B) {
	s := NewSMSPScheduler()
	for range 64 {
		w := trace.WarpTrace{}
		for range 32 {
			w.Instructions = append(w.Instructions, trace.InstructionTrace{OpCode: "IADD3", Mask: 1})
		}
		s.insertWarp(w)
	}
	pool := NewH100SMSPResourcePool()
	for range 2048 {
		pool.Reset()
		s.issueWarps(pool)
	}
	id := s.WarpUnits[63].ID
	ref := s.WarpUnits[63].InFlightPipelines[31].Ref()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = s.warp(id)
		_ = s.pipeline(ref)
	}
}
