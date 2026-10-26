package driver

import (
	"bytes"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
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
	if err := f.c.Resources.GlobalStorage.SaveCheckpoint(&storage); err != nil {
		t.Fatal(err)
	}
	if err := f.r.WriteHost(b, []uint32{99, 101}); err != nil {
		t.Fatal(err)
	}
	if err := f.r.LoadCheckpoint(&runtime); err != nil {
		t.Fatal(err)
	}
	if err := f.c.Resources.GlobalStorage.LoadCheckpoint(&storage); err != nil {
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
	if len(f.snapshot().Contexts[f.ctx-1].Buffers) != 0 {
		t.Fatal("freed allocation metadata retained")
	}
}
func TestHostCopyReadsStorageAtExecution(t *testing.T) {
	f := newFixture(t, false)
	dst := f.r.AllocateMemory(f.ctx, 8)
	b := f.r.AllocateHostBuffer(f.q, 8)
	if f.snapshot().Contexts[f.ctx-1].GPU != 1 {
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
	if err := f.c.Resources.GlobalStorage.SaveCheckpoint(&storage); err != nil {
		t.Fatal(err)
	}
	if err := f.r.WriteHost(b, []uint32{1, 2}); err != nil {
		t.Fatal(err)
	}
	if err := f.r.LoadCheckpoint(&runtime); err != nil {
		t.Fatal(err)
	}
	if err := f.c.Resources.GlobalStorage.LoadCheckpoint(&storage); err != nil {
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
		f.enqueue(f.q, gpuruntime.Command{Kind: gpuruntime.Noop})
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
	second := f.snapshot().Queues[q2-1].Pending
	if len(second) != 4 || second[0].Kind != gpuruntime.CopyH2D ||
		!bytes.Equal(readHostBytes(f.r, f.ctx, second[0].Src, uint64(second[0].ByteSize)), []byte{1, 2, 3, 4}) {
		t.Fatal("second queue relies on unfinished first queue upload")
	}
	if !bytes.Equal(readHostBytes(f.r, f.ctx, second[1].Src, uint64(second[1].ByteSize)), []byte{0x78, 0x56, 0x34, 0x12}) {
		t.Fatal("kernel args were not captured at enqueue")
	}
	if second[0].Dst == f.snapshot().Queues[f.q-1].Pending[5].Dst {
		t.Fatal("queues share an unready code allocation")
	}
	tickDriver(f.c)
	if f.snapshot().Queues[f.q-1].Pending[0].Kind != gpuruntime.Noop ||
		len(f.c.State.Executing) != 1 || f.c.State.Executing[0].Queue != q2 {
		t.Fatal("second upload did not progress independently while first queue was delayed")
	}
}
