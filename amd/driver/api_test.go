package driver

import (
	"bytes"
	"testing"

	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

func TestQueuesDrainThroughSimulation(t *testing.T) {
	for _, count := range []int{1, 3} {
		f := newFixture(t, false)
		for i := 0; i < count; i++ {
			f.enqueue(f.q, gpuruntime.Command{Kind: gpuruntime.Noop})
		}
		f.c.TickLater()
		if err := f.s.Run(); err != nil {
			t.Fatal(err)
		}
		if len(f.snapshot().Queues[f.q-1].Pending) != 0 || f.r.Unfinished()[0] != 0 {
			t.Fatal("queue did not drain")
		}
	}
}

func TestZeroByteCopyDrainsThroughDriver(t *testing.T) {
	f := newFixture(t, false)
	f.r.EnqueueMemCopyH2D(f.q, 0, 0, 0)
	marker := f.r.EnqueueRecordEvent(f.q)
	var runtime, component bytes.Buffer
	if err := f.r.SaveCheckpoint(&runtime); err != nil {
		t.Fatal(err)
	}
	if err := f.c.SaveCheckpoint(&component); err != nil {
		t.Fatal(err)
	}
	if err := f.r.LoadCheckpoint(&runtime); err != nil {
		t.Fatal(err)
	}
	if err := f.c.LoadCheckpoint(&component); err != nil {
		t.Fatal(err)
	}
	f.c.TickLater()
	if err := f.s.Run(); err != nil {
		t.Fatal(err)
	}
	if f.r.Unfinished()[0] != 0 || len(f.c.State.Executing) != 0 || len(f.snapshot().Contexts[f.ctx-1].Buffers) != 0 {
		t.Fatal("zero-byte copy retained work or allocation")
	}
	if _, err := f.r.EventTime(marker); err != nil {
		t.Fatal(err)
	}
	if err := f.r.ReleaseEvent(marker); err != nil {
		t.Fatal(err)
	}
}
