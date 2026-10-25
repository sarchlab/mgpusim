package driver

import (
	"github.com/sarchlab/akita/v5/mem"
	"testing"
)

func TestQueuesDrainThroughSimulation(t *testing.T) {
	for _, count := range []int{1, 3} {
		f := newFixture(t, false)
		for i := 0; i < count; i++ {
			f.r.enqueue(f.q, Command{Kind: noop})
		}
		f.c.TickLater()
		if err := f.s.Run(); err != nil {
			t.Fatal(err)
		}
		if len(f.r.queue(f.q).Pending) != 0 || f.r.Unfinished()[0] != 0 {
			t.Fatal("queue did not drain")
		}
	}
}
func TestAllocationMetadata(t *testing.T) {
	for _, unified := range []bool{false, true} {
		f := newFixture(t, false)
		var p Ptr
		if unified {
			p = f.r.AllocateUnifiedMemory(f.ctx, mem.MB)
		} else {
			p = f.r.AllocateMemory(f.ctx, mem.MB)
		}
		bufs := f.r.context(f.ctx).Buffers
		if len(bufs) != 1 || bufs[0].Address != p || bufs[0].Size != mem.MB || bufs[0].Dirty || bufs[0].Freed {
			t.Fatalf("allocation metadata: %+v", bufs)
		}
	}
}
