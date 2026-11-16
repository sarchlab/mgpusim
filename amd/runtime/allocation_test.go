package runtime

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
)

func TestAllocationMetadata(t *testing.T) {
	for _, unified := range []bool{false, true} {
		f := newRuntimeFixture(t)
		var p Ptr
		if unified {
			p = f.r.AllocateUnifiedMemory(f.ctx, mem.MB)
		} else {
			p = f.r.AllocateMemory(f.ctx, mem.MB)
		}
		bufs := f.r.context(f.ctx).Buffers
		if len(bufs) != 1 || bufs[0].Address != p || bufs[0].Size != mem.MB || bufs[0].Dirty {
			t.Fatalf("allocation metadata: %+v", bufs)
		}
	}
}
