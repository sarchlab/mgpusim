package runtime

import (
	"bytes"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

func TestZeroByteStagingCompletesAfterRestore(t *testing.T) {
	f := newRuntimeFixture(t)
	f.r.stageUpload(f.q, 0, []byte{})
	marker := f.r.EnqueueRecordEvent(f.q)
	if len(f.r.queue(f.q).Pending[0].Owned) != 0 {
		t.Fatal("zero address was owned")
	}
	var saved bytes.Buffer
	if err := f.r.SaveCheckpoint(&saved); err != nil {
		t.Fatal(err)
	}
	if err := f.r.LoadCheckpoint(&saved); err != nil {
		t.Fatal(err)
	}
	cmd, ok := f.r.TakeCommand(f.q)
	if !ok {
		t.Fatal("zero-byte upload missing")
	}
	f.r.CompleteCommand(f.q, cmd, 0)
	f.r.CompleteEvents(f.q, 0)
	if f.r.Unfinished()[0] != 0 || len(f.r.queue(f.q).Pending) != 0 {
		t.Fatal("zero-byte upload did not complete")
	}
	if _, err := f.r.EventTime(marker); err != nil {
		t.Fatal("following marker did not complete", err)
	}
	if err := f.r.ReleaseEvent(marker); err != nil {
		t.Fatal(err)
	}
	if len(f.r.context(f.ctx).Buffers) != 0 {
		t.Fatal("zero-byte staging allocated memory")
	}
}

func TestZeroArgumentDispatchKeepsLDSAndNoKernargAfterRestore(t *testing.T) {
	f := newRuntimeFixture(t)
	kernel := &insts.KernelCodeObject{
		Data:                 []byte{0, 0, 0x81, 0xbf},
		KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{GroupSegmentByteSize: 128},
	}
	f.r.EnqueueLaunchKernel(f.q, kernel, [3]uint32{1, 1, 1}, [3]uint16{1, 1, 1}, nil)
	var image bytes.Buffer
	if err := f.r.SaveCheckpoint(&image); err != nil {
		t.Fatal(err)
	}
	if err := f.r.LoadCheckpoint(&image); err != nil {
		t.Fatal(err)
	}
	pending := f.r.queue(f.q).Pending
	// Only code upload, packet upload, and dispatch should be queued.
	if len(pending) != 3 || pending[2].Kind != LaunchKernel || len(pending[2].Packets) != 1 {
		t.Fatal("unexpected restored zero-argument dispatch")
	}
	packet := pending[2].Packets[0].Packet
	if packet.KernargAddress != 0 || packet.GroupSegmentSize != 128 {
		t.Fatalf("unexpected packet kernarg address %x or LDS size %d", packet.KernargAddress, packet.GroupSegmentSize)
	}
}
