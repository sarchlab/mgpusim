package driver

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

func TestReleasedEventsCannotResurrectAfterRestore(t *testing.T) {
	f := newFixture(t, false)
	for i := 0; i < 12; i++ {
		released := f.r.EnqueueRecordEvent(f.q)
		live := f.r.EnqueueRecordEvent(f.q)
		if err := f.r.ReleaseEvent(released); err != nil {
			t.Fatal(err)
		}
		var checkpoint bytes.Buffer
		if err := f.r.SaveCheckpoint(&checkpoint); err != nil {
			t.Fatal(err)
		}
		if err := f.r.LoadCheckpoint(&checkpoint); err != nil {
			t.Fatal(err)
		}
		tickDriver(f.c)
		if _, err := f.r.EventTime(released); err == nil {
			t.Fatal("released marker resurrected")
		}
		first, err := f.r.EventTime(live)
		if err != nil {
			t.Fatal(err)
		}
		again, err := f.r.EventTime(live)
		if err != nil || again != first {
			t.Fatal("event read was not repeatable")
		}
		if err := f.r.ReleaseEvent(live); err != nil {
			t.Fatal(err)
		}
		checkpoint.Reset()
		if err := f.r.SaveCheckpoint(&checkpoint); err != nil {
			t.Fatal(err)
		}
		if err := f.r.LoadCheckpoint(&checkpoint); err != nil {
			t.Fatal(err)
		}
		if _, err := f.r.EventTime(live); err == nil {
			t.Fatal("completed released event restored")
		}
		if err := f.r.ReleaseEvent(live); err == nil {
			t.Fatal("duplicate release accepted")
		}
		if len(f.r.state.Events) != 0 || f.r.Unfinished()[0] != 0 {
			t.Fatal("event lifecycle retained state")
		}
	}
}
func TestUnifiedLaunchSkipsEmptyGPUSelection(t *testing.T) {
	f := newFixture(t, false)
	packet := kernels.PacketValue{Packet: kernels.HsaKernelDispatchPacket{
		GridSizeX: 64, GridSizeY: 1, GridSizeZ: 1, WorkgroupSizeX: 64, WorkgroupSizeY: 1, WorkgroupSizeZ: 1,
	}}
	f.r.enqueue(f.q, Command{
		Kind: launchKernel, GPUs: []int{1, 2},
		Packets: []kernels.PacketValue{packet, packet}, PacketAddresses: []Ptr{0, 0},
	})
	tickDriver(f.c)
	requests := f.c.State.Executing[0].Requests
	if len(requests) != 1 || requests[0].Dst != f.c.Spec.GPUs[0] {
		t.Fatal("idle unified GPU received launch", requests)
	}
	selection := requests[0].Payload.(protocol.LaunchKernelReq).Selection
	if selection.First != 0 || selection.End != 1 {
		t.Fatal(selection)
	}
	f.reply(requests[0], protocol.LaunchKernelRsp{})
	if f.r.Unfinished()[0] != 0 {
		t.Fatal("single active partition did not complete")
	}
}
func TestCodeCacheHashIndexRebuildAndCollisionCheck(t *testing.T) {
	f := newFixture(t, false)
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

//nolint:funlen // Keep the partial-copy, restore, and buffer-reuse lifecycle together.
func TestPartialD2HStorageAndContinuationRestore(t *testing.T) {
	f := newFixture(t, false)
	host := f.r.AllocateHostBuffer(f.q, 8)
	first := f.request(protocol.MemCopyD2HReq{ByteSize: 4, DstOffset: 0})
	second := f.request(protocol.MemCopyD2HReq{ByteSize: 4, DstOffset: 4})
	f.running(Command{Kind: copyD2H, Dst: host.Address, ByteSize: 8}, []messaging.Msg{first, second})
	f.reply(first, protocol.MemCopyD2HRsp{Data: []byte{3, 0, 0, 0}})
	var runtime, component, storage bytes.Buffer
	if err := f.r.SaveCheckpoint(&runtime); err != nil {
		t.Fatal(err)
	}
	if err := f.c.SaveCheckpoint(&component); err != nil {
		t.Fatal(err)
	}
	if err := f.r.storage.SaveCheckpoint(&storage); err != nil {
		t.Fatal(err)
	}
	if err := f.r.WriteHost(host, []uint32{99, 99}); err != nil {
		t.Fatal(err)
	}
	if err := f.r.LoadCheckpoint(&runtime); err != nil {
		t.Fatal(err)
	}
	if err := f.c.LoadCheckpoint(&component); err != nil {
		t.Fatal(err)
	}
	if err := f.r.storage.LoadCheckpoint(&storage); err != nil {
		t.Fatal(err)
	}
	out := make([]uint32, 2)
	if err := f.r.ReadHost(host, out); err != nil {
		t.Fatal(err)
	}
	if out[0] != 3 || out[1] != 0 || f.r.Unfinished()[0] != 1 {
		t.Fatal("partial bytes or in-flight ownership lost", out)
	}
	f.reply(second, protocol.MemCopyD2HRsp{Data: []byte{5, 0, 0, 0}})
	if err := f.r.ReadHost(host, out); err != nil {
		t.Fatal(err)
	}
	if out[0] != 3 || out[1] != 5 || f.r.Unfinished()[0] != 0 {
		t.Fatal("restored copy did not complete", out)
	}
	// Reuse the completed CPU destination as the next upload source.
	if err := f.r.WriteHost(host, []uint32{7, 11}); err != nil {
		t.Fatal(err)
	}
	dst := f.r.AllocateMemory(f.ctx, 8)
	f.r.EnqueueMemCopyH2D(f.q, dst, host.Address, 8)
	tickDriver(f.c)
	request := f.c.State.Executing[0].Requests[0]
	if !bytes.Equal(request.Payload.(protocol.MemCopyH2DReq).SrcBuffer, []byte{7, 0, 0, 0, 11, 0, 0, 0}) {
		t.Fatal("reused buffer uploaded stale data")
	}
	f.reply(request, protocol.GeneralRsp{})
	if err := f.r.ReadHost(host, out); err != nil {
		t.Fatal(err)
	}
	if out[0] != 7 || out[1] != 11 {
		t.Fatal("reused buffer data", out)
	}
	if err := f.r.FreeMemory(host.Context, host.Address); err != nil {
		t.Fatal(err)
	}
	if err := f.r.FreeMemory(f.ctx, dst); err != nil {
		t.Fatal(err)
	}
	if len(f.r.context(f.ctx).Buffers) != 0 {
		t.Fatal("host allocation metadata accumulated")
	}
}

func TestZeroByteStagingCompletesAfterRestore(t *testing.T) {
	f := newFixture(t, false)
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
	tickDriver(f.c)
	if f.r.Unfinished()[0] != 0 || len(f.c.State.Executing) != 0 {
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
