package runtime

import (
	"bytes"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

func TestCodeCacheHashIndexRebuildAndCollisionCheck(t *testing.T) {
	f := newRuntimeFixture(t)
	first := &insts.KernelCodeObject{Data: []byte{1, 2, 3, 4},
		KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{KernargSegmentByteSize: 4}}
	second := &insts.KernelCodeObject{Data: []byte{5, 6, 7, 8},
		KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{KernargSegmentByteSize: 4}}
	args := uint32(1)
	launch := func(c *insts.KernelCodeObject) {
		f.r.EnqueueLaunchKernel(f.q, c, [3]uint32{64, 1, 1}, [3]uint16{64, 1, 1}, &args)
	}
	launch(first)
	launch(second)
	key := codeKey{f.q, f.r.context(f.ctx).PID, 1, hashCode(insts.CodeObjectSnapshot(first))}
	f.r.codeIndex[key] = []int{1, 0} // Deliberately exercise a hash bucket collision.
	before := len(f.r.queue(f.q).Pending)
	launch(first)
	if len(f.r.state.Codes) != 2 || len(f.r.queue(f.q).Pending)-before != 3 {
		t.Fatal("cache did not reuse matching code")
	}
	last := f.r.queue(f.q).Pending[len(f.r.queue(f.q).Pending)-1]
	if Ptr(last.Packets[0].Packet.KernelObject) != f.r.state.Codes[0].Address {
		t.Fatal("collision matched unequal code")
	}
	var saved bytes.Buffer
	if err := f.r.SaveCheckpoint(&saved); err != nil {
		t.Fatal(err)
	}
	if err := f.r.LoadCheckpoint(&saved); err != nil {
		t.Fatal(err)
	}
	launch(second)
	if len(f.r.state.Codes) != 2 {
		t.Fatal("restored cache index did not reuse canonical entries")
	}
	changed := insts.CodeObjectSnapshot(first)
	changed.Meta.KernargSegmentByteSize++
	if hashCode(changed) == hashCode(insts.CodeObjectSnapshot(first)) {
		t.Fatal("hash omitted code metadata")
	}
}
