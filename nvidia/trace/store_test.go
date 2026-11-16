package trace

import (
	"bytes"
	"sync"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
)

func storeTestKernel() *KernelTrace {
	inst := InstructionTrace{OpCode: "IADD3", SrcRegs: []Register{{"R1"}}, DestRegs: []Register{{"R2"}},
		MemAddressSuffix2: []int32{4}}
	warp := WarpTrace{Instructions: []InstructionTrace{inst}}
	return &KernelTrace{ID: "same-file", Threadblocks: []ThreadblockTrace{{Warps: []WarpTrace{warp}}}}
}

func TestStoreSubmissionIsolationLifetimeAndRewind(t *testing.T) {
	s := NewStore(sim.MakeBuilder().Build(), "Traces")
	input := storeTestKernel()
	first := s.Add(input)
	second := s.Add(input)
	ref := WarpRef{Kernel: first}
	input.Threadblocks[0].Warps[0].Instructions[0].SrcRegs[0].Name = "changed"
	input.Threadblocks[0].Warps[0].Instructions[0].DestRegs[0].Name = "changed"
	input.Threadblocks[0].Warps[0].Instructions[0].MemAddressSuffix2[0] = 99
	input.Threadblocks[0].Warps[0].Instructions[0].OpCode = "changed"
	got := s.Warp(ref).Instructions[0]
	if got.OpCode != "IADD3" || got.SrcRegs[0].Name != "R1" || got.DestRegs[0].Name != "R2" ||
		got.MemAddressSuffix2[0] != 4 {
		t.Fatal("submission aliases caller trace")
	}
	var saved bytes.Buffer
	if err := s.SaveCheckpoint(&saved); err != nil {
		t.Fatal(err)
	}
	s.Release(first)
	if len(s.state.Kernels) != 1 || s.Kernel(second).ID != "same-file" {
		t.Fatal("release affected another launch")
	}
	s.Release(second)
	if len(s.state.Kernels) != 0 {
		t.Fatal("completed traces retained")
	}
	// Reuse the same store after ID generation and release, then also restore a
	// fresh instance. The archive itself supplies every trace byte.
	for _, target := range []*Store{s, NewStore(sim.MakeBuilder().Build(), "Traces")} {
		if err := target.LoadCheckpoint(bytes.NewReader(saved.Bytes())); err != nil {
			t.Fatal(err)
		}
		if target.Warp(ref).Instructions[0].SrcRegs[0].Name != "R1" {
			t.Fatal("rewind retained stale trace")
		}
		target.Release(first)
		target.Release(second)
		if len(target.state.Kernels) != 0 {
			t.Fatal("rewind leaked traces")
		}
		if id := target.Add(storeTestKernel()); id != 3 {
			t.Fatalf("restored launch sequence: %d", id)
		}
	}
}

func TestStoreConcurrentIndependentLaunches(t *testing.T) {
	s := NewStore(sim.MakeBuilder().Build(), "Traces")
	id := s.Add(storeTestKernel())
	var workers sync.WaitGroup
	workers.Go(func() {
		for range 1000 {
			if s.Warp(WarpRef{Kernel: id}).Instructions[0].OpCode != "IADD3" {
				t.Error("live launch changed")
			}
		}
	})
	workers.Go(func() {
		for range 1000 {
			other := s.Add(storeTestKernel())
			s.Release(other)
		}
	})
	workers.Wait()
	s.Release(id)
}
