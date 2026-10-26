package driver

import (
	"bytes"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

func TestRestoredCPUBufferBytes(t *testing.T) {
	f := newFixture(t, false)
	b := f.r.AllocateHostBuffer(f.q, 8)
	if err := f.r.WriteHost(b, []uint32{7, 9}); err != nil {
		t.Fatal(err)
	}
	var runtime, storage bytes.Buffer
	if err := f.r.SaveCheckpoint(&runtime); err != nil {
		t.Fatal(err)
	}
	if err := f.r.storage.SaveCheckpoint(&storage); err != nil {
		t.Fatal(err)
	}
	if err := f.r.WriteHost(b, []uint32{99, 101}); err != nil {
		t.Fatal(err)
	}
	if err := f.r.LoadCheckpoint(&runtime); err != nil {
		t.Fatal(err)
	}
	if err := f.r.storage.LoadCheckpoint(&storage); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []any{nil, []uint32{1}, uint64(1), (*uint64)(nil)} {
		if err := f.r.ReadHost(b, invalid); err == nil {
			t.Fatalf("accepted invalid destination %T", invalid)
		}
	}
	for i := 0; i < 2; i++ {
		output := make([]uint32, 2)
		if err := f.r.ReadHost(b, output); err != nil {
			t.Fatal(err)
		}
		if output[0] != 7 || output[1] != 9 {
			t.Fatal(output)
		}
	}
	if err := f.r.FreeMemory(b.Context, b.Address); err != nil {
		t.Fatal(err)
	}
	if len(f.r.context(f.ctx).Buffers) != 0 {
		t.Fatal("freed allocation metadata retained")
	}
}
func TestHostCopyReadsStorageAtExecution(t *testing.T) {
	f := newFixture(t, false)
	dst := f.r.AllocateMemory(f.ctx, 8)
	b := f.r.AllocateHostBuffer(f.q, 8)
	if f.r.context(f.ctx).GPU != 1 {
		t.Fatal("CPU allocation changed selected GPU")
	}
	if err := f.r.WriteHost(b, []uint32{11, 13}); err != nil {
		t.Fatal(err)
	}
	f.r.EnqueueMemCopyH2D(f.q, dst, b.Address, 8)
	// This intentionally violates source lifetime discipline. There is no
	// occupancy guard: serial execution deterministically reads overwritten data.
	if err := f.r.WriteHost(b, []uint32{99, 103}); err != nil {
		t.Fatal(err)
	}
	var runtime, storage bytes.Buffer
	if err := f.r.SaveCheckpoint(&runtime); err != nil {
		t.Fatal(err)
	}
	if err := f.r.storage.SaveCheckpoint(&storage); err != nil {
		t.Fatal(err)
	}
	if err := f.r.WriteHost(b, []uint32{1, 2}); err != nil {
		t.Fatal(err)
	}
	if err := f.r.LoadCheckpoint(&runtime); err != nil {
		t.Fatal(err)
	}
	if err := f.r.storage.LoadCheckpoint(&storage); err != nil {
		t.Fatal(err)
	}
	tickDriver(f.c)
	got := f.c.State.Executing[0].Requests[0].Payload.(protocol.MemCopyH2DReq).SrcBuffer
	if !bytes.Equal(got, []byte{99, 0, 0, 0, 103, 0, 0, 0}) {
		t.Fatal(got)
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
	if len(second) != 4 || second[0].Kind != copyH2D ||
		!bytes.Equal(f.r.readBytes(f.ctx, second[0].Src, uint64(second[0].ByteSize)), []byte{1, 2, 3, 4}) {
		t.Fatal("second queue relies on unfinished first queue upload")
	}
	if !bytes.Equal(f.r.readBytes(f.ctx, second[1].Src, uint64(second[1].ByteSize)), []byte{0x78, 0x56, 0x34, 0x12}) {
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
