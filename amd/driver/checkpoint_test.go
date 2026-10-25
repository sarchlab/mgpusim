package driver

import (
	"bytes"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"testing"
)

func TestRestoredResultBytes(t *testing.T) {
	f := newFixture(t, false)
	pending := f.r.EnqueueMemCopyD2H(f.q, 4096, 8)
	f.r.state.Results[23] = []byte{7, 0, 0, 0, 9, 0, 0, 0}
	var saved bytes.Buffer
	if err := f.r.SaveCheckpoint(&saved); err != nil {
		t.Fatal(err)
	}
	if err := f.r.LoadCheckpoint(&saved); err != nil {
		t.Fatal(err)
	}
	for _, id := range []CommandID{pending, 23} {
		for _, invalid := range []any{nil, []uint32{1}, uint64(1), (*uint64)(nil)} {
			if err := f.r.ReadResult(id, invalid); err == nil {
				t.Fatalf("accepted invalid destination %T", invalid)
			}
		}
	}
	output := []uint32{101, 103}
	if err := f.r.ReadResult(pending, output); err == nil || output[0] != 101 || output[1] != 103 {
		t.Fatal("pending read changed host output")
	}
	cmd, _ := f.r.take(f.q)
	cmd.Data = []byte{3, 0, 0, 0, 5, 0, 0, 0}
	f.r.complete(f.q, cmd, 0)
	if err := f.r.ReadResult(pending, &output); err != nil {
		t.Fatal(err)
	}
	if output[0] != 3 || output[1] != 5 {
		t.Fatal(output)
	}
	completed := make([]uint32, 2)
	if err := f.r.ReadResult(23, completed); err != nil {
		t.Fatal(err)
	}
	if completed[0] != 7 || completed[1] != 9 {
		t.Fatal(completed)
	}
	if len(f.r.state.Results) != 0 {
		t.Fatal("consumed results retained")
	}
	if err := f.r.ReadResult(23, completed); err == nil {
		t.Fatal("delivered twice")
	}
}
func TestHostCopySnapshotOwnsBytes(t *testing.T) {
	f := newFixture(t, false)
	source := []uint32{11, 13}
	f.r.EnqueueMemCopyH2D(f.q, 4096, source)
	source[0] = 99
	var saved bytes.Buffer
	if err := f.r.SaveCheckpoint(&saved); err != nil {
		t.Fatal(err)
	}
	if err := f.r.LoadCheckpoint(&saved); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f.r.queue(f.q).Pending[0].Data, []byte{11, 0, 0, 0, 13, 0, 0, 0}) {
		t.Fatal("queue retained host source reference")
	}
}

func TestQueueCodeUploadDoesNotDependOnAnotherQueue(t *testing.T) {
	f := newFixture(t, false)
	q2 := f.r.CreateCommandQueue(f.ctx)
	// An older queue cannot confer code readiness while its own upload is
	// blocked behind earlier work. The second queue must own its upload.
	for i := 0; i < 5; i++ {
		f.r.enqueue(f.q, Command{Kind: noop})
	}
	code := &insts.KernelCodeObject{
		Data:                 []byte{1, 2, 3, 4},
		KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{KernargSegmentByteSize: 4},
	}
	args := uint32(0x12345678)
	f.r.EnqueueLaunchKernel(f.q, code, [3]uint32{64, 1, 1}, [3]uint16{64, 1, 1}, &args)
	f.r.EnqueueLaunchKernel(q2, code, [3]uint32{64, 1, 1}, [3]uint16{64, 1, 1}, &args)
	args = 99
	code.Data[0] = 99
	second := f.r.queue(q2).Pending
	if len(second) != 4 || second[0].Kind != copyH2D || !bytes.Equal(second[0].Data, []byte{1, 2, 3, 4}) {
		t.Fatal("second queue relies on unfinished first queue upload")
	}
	if !bytes.Equal(second[1].Data, []byte{0x78, 0x56, 0x34, 0x12}) {
		t.Fatal("kernel args were not captured at enqueue")
	}
	if second[0].Dst == f.r.queue(f.q).Pending[5].Dst {
		t.Fatal("queues share an unready code allocation")
	}
	tickDriver(f.c)
	if f.r.queue(f.q).Pending[0].Kind != noop || len(f.c.State.Executing) != 1 || f.c.State.Executing[0].Queue != q2 {
		t.Fatal("second upload did not progress independently while first queue was delayed")
	}
}
