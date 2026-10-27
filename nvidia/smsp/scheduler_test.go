package smsp

import (
	"bytes"
	"encoding/gob"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

func lookupTestWarp() trace.WarpTrace {
	return trace.WarpTrace{Instructions: []trace.InstructionTrace{{OpCode: "IADD3", Mask: 1}}}
}

func TestSchedulerLookupsFollowGrowthCompactionAndRestore(t *testing.T) {
	s := NewSMSPScheduler()
	store := trace.NewStore(sim.MakeBuilder().Build(), "Traces")
	for range 64 {
		s.insertWarp(testWarpRef(store, lookupTestWarp()), store)
	}
	for i := range s.WarpUnits {
		w := &s.WarpUnits[i]
		for j := range 8 {
			w.InFlightPipelines = append(w.InFlightPipelines, PipelineInstance{WarpID: w.ID, InstIndex: uint64(j)})
		}
	}
	var snapshot bytes.Buffer
	if err := gob.NewEncoder(&snapshot).Encode(s); err != nil {
		t.Fatal(err)
	}
	c := &SMSPController{Comp: &Comp{}}
	c.State.Scheduler = *s
	retireTestPipelines(c)
	s = &c.State.Scheduler
	assertLookupPanics(t, func() { s.warp(1) })
	checkCompactedLookups(t, s)
	s.insertWarp(testWarpRef(store, lookupTestWarp()), store)
	if s.warp(65).ID != 65 {
		t.Fatal("insertion after retirement broke ordering")
	}
	// Replace canonical state using the same IDs; lookups must resolve restored values.
	var restored SMSPSWarpScheduler
	if err := gob.NewDecoder(&snapshot).Decode(&restored); err != nil {
		t.Fatal(err)
	}
	c.State.Scheduler = restored
	if c.State.Scheduler.warp(1).ID != 1 || len(c.State.Scheduler.warp(2).InFlightPipelines) != 8 {
		t.Fatal("lookup retained compacted state after restore")
	}
	c.State.Scheduler.pipeline(PipelineRef{WarpID: 2, InstIndex: 0}).PC = 17
	if c.State.Scheduler.WarpUnits[1].InFlightPipelines[0].PC != 17 {
		t.Fatal("lookup does not reference canonical state")
	}
}

func retireTestPipelines(c *SMSPController) {
	// Retire all pipelines in one warp and alternating pipelines in the others.
	for wi := range c.State.Scheduler.WarpUnits {
		w := &c.State.Scheduler.WarpUnits[wi]
		w.UnfinishedInstsCount = 8
		for pi := range w.InFlightPipelines {
			w.InFlightPipelines[pi].Done = wi == 0 || pi%2 == 0
			w.InFlightPipelines[pi].SrcRegsReleased = true
			w.InFlightPipelines[pi].DstRegsReleased = true
		}
	}
	c.tickInFlightPipelines()
	c.collectFinishedWarps()
}

func checkCompactedLookups(t *testing.T, s *SMSPSWarpScheduler) {
	t.Helper()
	for _, w := range s.WarpUnits {
		if s.warp(w.ID).ID != w.ID {
			t.Fatal("warp lookup failed after compaction")
		}
		for _, p := range w.InFlightPipelines {
			if s.pipeline(p.Ref()).InstIndex != p.InstIndex {
				t.Fatal("pipeline lookup failed after compaction")
			}
		}
		assertLookupPanics(t, func() { s.pipeline(PipelineRef{WarpID: w.ID, InstIndex: 0}) })
	}
}

func assertLookupPanics(t *testing.T, lookup func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("retired object resolved")
		}
	}()
	lookup()
}

func testWarpRef(store *trace.Store, w trace.WarpTrace) trace.WarpRef {
	id := store.Add(&trace.KernelTrace{Threadblocks: []trace.ThreadblockTrace{{Warps: []trace.WarpTrace{w}}}})
	return trace.WarpRef{Kernel: id}
}
