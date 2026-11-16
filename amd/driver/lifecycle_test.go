package driver

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
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
		if len(f.snapshot().Events) != 0 || f.r.Unfinished()[0] != 0 {
			t.Fatal("event lifecycle retained state")
		}
	}
}
func TestUnifiedLaunchSkipsEmptyGPUSelection(t *testing.T) {
	f := newFixture(t, false)
	packet := kernels.PacketValue{Packet: kernels.HsaKernelDispatchPacket{
		GridSizeX: 64, GridSizeY: 1, GridSizeZ: 1, WorkgroupSizeX: 64, WorkgroupSizeY: 1, WorkgroupSizeZ: 1,
	}}
	f.enqueue(f.q, gpuruntime.Command{
		Kind: gpuruntime.LaunchKernel, GPUs: []int{1, 2},
		Packets: []kernels.PacketValue{packet, packet}, PacketAddresses: []gpuruntime.Ptr{0, 0},
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

//nolint:funlen // Keep the partial-copy, restore, and buffer-reuse lifecycle together.
func TestPartialD2HStorageAndContinuationRestore(t *testing.T) {
	f := newFixture(t, false)
	host := f.r.AllocateHostBuffer(f.q, 8)
	first := f.request(protocol.MemCopyD2HReq{ByteSize: 4, DstOffset: 0})
	second := f.request(protocol.MemCopyD2HReq{ByteSize: 4, DstOffset: 4})
	f.running(gpuruntime.Command{Kind: gpuruntime.CopyD2H, Dst: host.Address, ByteSize: 8}, []messaging.Msg{first, second})
	f.reply(first, protocol.MemCopyD2HRsp{Data: []byte{3, 0, 0, 0}})
	var runtime, component, storage bytes.Buffer
	if err := f.r.SaveCheckpoint(&runtime); err != nil {
		t.Fatal(err)
	}
	if err := f.c.SaveCheckpoint(&component); err != nil {
		t.Fatal(err)
	}
	if err := f.c.Resources.GlobalStorage.SaveCheckpoint(&storage); err != nil {
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
	if err := f.c.Resources.GlobalStorage.LoadCheckpoint(&storage); err != nil {
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
	if len(f.snapshot().Contexts[f.ctx-1].Buffers) != 0 {
		t.Fatal("host allocation metadata accumulated")
	}
}
