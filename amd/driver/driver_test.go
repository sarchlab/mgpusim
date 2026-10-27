package driver

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// noopConn leaves outgoing messages available to protocol assertions.
type noopConn struct{ hooking.HookableBase }

func (c *noopConn) Name() string                   { return "NoopConn" }
func (c *noopConn) BindPort(p messaging.Port)      { p.BindConnection(c) }
func (c *noopConn) Unplug(messaging.Port)          {}
func (c *noopConn) NotifyAvailable(messaging.Port) {}
func (c *noopConn) NotifySend(messaging.Port)      {}

type fixture struct {
	s    *sim.Simulation
	c    *Comp
	r    *gpuruntime.Runtime
	ctx  gpuruntime.Context
	q    gpuruntime.CommandQueue
	p    *vm.PageTable
	port messaging.Port
}

func newFixture(t *testing.T, mixed bool) *fixture {
	t.Helper()
	return newConfiguredFixture(t, mixed, func(*Spec) {})
}
func newConfiguredFixture(t *testing.T, mixed bool, configure func(*Spec)) *fixture {
	t.Helper()
	return newRuntimeConfiguredFixture(t, mixed, configure, func(*gpuruntime.RuntimeSpec) {})
}
func newRuntimeConfiguredFixture(
	t *testing.T, mixed bool, configure func(*Spec), configureRuntime func(*gpuruntime.RuntimeSpec),
) *fixture {
	t.Helper()
	f := &fixture{s: sim.MakeBuilder().Build()}
	pageBits := uint64(12)
	if mixed {
		pageBits = 11
	}
	f.p = vm.NewPageTable(pageBits)
	spec := DefaultSpec()
	runtimeSpec := gpuruntime.DefaultRuntimeSpec()
	runtimeSpec.Log2PageSize = pageBits
	spec.H2DCycles = 1
	spec.D2HCycles = 1
	for i := 0; i < 2; i++ {
		spec.GPUs = append(spec.GPUs, messaging.RemotePort(fmt.Sprintf("GPU[%d].CommandProcessor", i+1)))
		runtimeSpec.Devices = append(runtimeSpec.Devices, gpuruntime.DeviceProperties{CUCount: 4, DRAMSize: 4 * mem.GB})
	}
	configure(&spec)
	configureRuntime(&runtimeSpec)
	storage := mem.MakeStorageBuilder().WithSimulation(f.s).WithCapacity(12 * mem.GB).Build("GlobalStorage")
	f.r = gpuruntime.NewRuntime(f.s, "Runtime", runtimeSpec, f.p, storage)
	f.c = MakeBuilder().WithSimulation(f.s).WithSpec(spec).WithResources(Resources{
		Runtime:       f.r,
		GlobalStorage: storage,
		PageTable:     f.p,
	}).Build("Driver")
	f.port = twowaybuffered.MakeBuilder().WithSimulation(f.s).WithSpec(twowaybuffered.Spec{
		IncomingCapacity: 16,
		OutgoingCapacity: 16,
	}).Build()
	f.c.BindPort(GPUPortName, f.port)
	(&noopConn{}).BindPort(f.port)
	if err := f.s.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	f.ctx = f.r.Init()
	f.q = f.r.CreateCommandQueue(f.ctx)
	return f
}
func tickDriver(c *Comp) bool { return c.Middlewares.Execute.Tick() }
func (f *fixture) running(cmd gpuruntime.Command, reqs []messaging.Msg) gpuruntime.CommandID {
	id := f.enqueue(f.q, cmd)
	cmd, _ = f.r.TakeCommand(f.q)
	f.c.State.Executing = append(
		f.c.State.Executing,
		Execution{Queue: f.q, Command: cmd, Requests: append([]messaging.Msg(nil), reqs...)},
	)
	return id
}
func (f *fixture) request(payload any) messaging.Msg {
	return messaging.Msg{ID: f.c.NewID(), Src: f.port.AsRemote(), Dst: f.c.Spec.GPUs[0], Payload: payload}
}
func (f *fixture) reply(req messaging.Msg, payload any) {
	f.port.Deliver(messaging.Msg{ID: f.c.NewID(), RspTo: req.ID, Src: req.Dst, Dst: req.Src, Payload: payload})
	tickDriver(f.c)
}

//nolint:gocognit // Keep each recovery/protocol scenario with its complete assertions.
func TestCopyRequestPartitioning(t *testing.T) {
	for _, read := range []bool{false, true} {
		t.Run(fmt.Sprint("D2H=", read), func(t *testing.T) {
			f := newFixture(t, true)
			host := f.r.AllocateHostBuffer(f.q, 0x2200)
			sizes := []uint64{0x800, 0x800, 0x1000, 0x1000}
			gpuAddr := f.r.AllocateMemory(f.ctx, 0x3000)
			page, ok := f.p.Find(f.r.QueuePID(f.q), uint64(gpuAddr))
			if !ok {
				t.Fatal("GPU allocation missing")
			}
			v, p := uint64(0x200000000), page.PAddr
			for _, size := range sizes {
				f.p.Insert(vm.Page{PID: 1, VAddr: v, PAddr: p, PageSize: size, Valid: true})
				v += size
				p += size
			}
			count := 4
			if read {
				count = 1
			}

			if read {
				f.r.EnqueueMemCopyD2H(f.q, host.Address, gpuruntime.Ptr(0x200000100), 4)
			} else {
				f.r.EnqueueMemCopyH2D(f.q, gpuruntime.Ptr(0x200000100), host.Address, 0x2200)
			}
			for i := 0; i < 3; i++ {
				tickDriver(f.c)
			}
			if len(f.snapshot().Queues[f.q-1].Pending) != 0 || f.r.Unfinished()[0] != 1 || len(f.c.State.Executing) != 1 {
				t.Fatal("command did not transfer exactly once to Driver.State")
			}
			e := f.c.State.Executing[0]
			for _, req := range e.Requests {
				if req.Dst != f.c.Spec.GPUs[0] {
					t.Fatal("partition routed to wrong physical owner", req.Dst)
				}
			}
			if len(e.Requests) != count || len(f.c.State.RequestsToSend) != count {
				t.Fatalf("requests: %+v", e)
			}
			if !read {
				want := []int{0x700, 0x800, 0x1000, 0x300}
				for i, req := range e.Requests {
					if got := len(req.Payload.(protocol.MemCopyH2DReq).SrcBuffer); got != want[i] {
						t.Fatalf("chunk %d=%d, want %d", i, got, want[i])
					}
				}
			}
		})
	}
}

//nolint:gocognit // Keep each recovery/protocol scenario with its complete assertions.
func TestCopyResponseOwnership(t *testing.T) {
	for _, read := range []bool{false, true} {
		for _, count := range []int{1, 2} {
			t.Run(fmt.Sprintf("D2H=%v/requests=%d", read, count), func(t *testing.T) {
				f := newFixture(t, false)
				host := f.r.AllocateHostBuffer(f.q, count*4)
				cmd := gpuruntime.Command{Kind: gpuruntime.CopyH2D, Dst: host.Address, ByteSize: count * 4}
				if read {
					cmd.Kind = gpuruntime.CopyD2H
					cmd.ByteSize = count * 4
				}
				reqs := make([]messaging.Msg, count)
				for i := range reqs {
					var p any = protocol.MemCopyH2DReq{DstAddress: uint64(0x100 + i*4)}
					if read {
						p = protocol.MemCopyD2HReq{SrcAddress: uint64(0x100 + i*4), ByteSize: 4, DstOffset: uint64(i * 4)}
					}
					reqs[i] = f.request(p)
				}
				f.running(cmd, reqs)
				for i, req := range reqs {
					var p any = protocol.GeneralRsp{}
					if read {
						p = protocol.MemCopyD2HRsp{Data: []byte{byte(i + 1), 0, 0, 0}}
					}
					f.reply(req, p)
					if i+1 < count {
						if len(f.c.State.Executing) != 1 || len(f.c.State.Executing[0].Requests) != count-i-1 ||
							f.r.Unfinished()[0] != 1 {
							t.Fatal("copy completed before all replies")
						}
						if read {
							partial := make([]uint32, count)
							if err := f.r.ReadHost(host, partial); err != nil {
								t.Fatal(err)
							}
							if partial[0] != 1 || partial[count-1] != 0 {
								t.Fatal("partial CPU storage", partial)
							}
						}
					}
				}
				if len(f.c.State.Executing) != 0 || f.r.Unfinished()[0] != 0 {
					t.Fatal("copy did not complete")
				}
				if read {
					out := make([]uint32, count)
					if err := f.r.ReadHost(host, out); err != nil {
						t.Fatal(err)
					}
					for i, v := range out {
						if v != uint32(i+1) {
							t.Fatalf("output %v", out)
						}
					}
				}
			})
		}
	}
}

//nolint:gocognit // Keep each recovery/protocol scenario with its complete assertions.
func TestCopyCompletesForEveryFlushResponseOrder(t *testing.T) {
	orders := [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {2, 0, 1}, {1, 2, 0}, {2, 1, 0}}
	for _, read := range []bool{false, true} {
		for _, order := range orders {
			t.Run(fmt.Sprintf("D2H=%v/%v", read, order), func(t *testing.T) {
				f := newFixture(t, false)
				payload := []byte{0x78, 0x56, 0x34, 0x12}
				host := f.r.AllocateHostBuffer(f.q, 4)
				cmd := gpuruntime.Command{Kind: gpuruntime.CopyH2D, Dst: host.Address, ByteSize: 4}
				var copyPayload any = protocol.MemCopyH2DReq{SrcBuffer: payload}
				if read {
					cmd.Kind = gpuruntime.CopyD2H
					cmd.ByteSize = 4
					copyPayload = protocol.MemCopyD2HReq{ByteSize: 4}
				}
				reqs := []messaging.Msg{f.request(copyPayload), f.request(protocol.FlushReq{}), f.request(protocol.FlushReq{})}
				f.running(cmd, reqs)
				f.enqueue(f.q, gpuruntime.Command{Kind: gpuruntime.Noop})
				for i, index := range order {
					var p any = protocol.GeneralRsp{}
					if read && index == 0 {
						p = protocol.MemCopyD2HRsp{Data: payload}
					}
					f.reply(reqs[index], p)
					if i < 2 {
						if f.r.Unfinished()[0] != 2 || len(f.snapshot().Queues[f.q-1].Pending) != 1 ||
							len(f.c.State.Executing[0].Requests) != 2-i {
							t.Fatal("flush/copy ownership released early")
						}
						if read {
							var out uint32
							if err := f.r.ReadHost(host, &out); err != nil {
								t.Fatal(err)
							}
							copied := false
							for _, done := range order[:i+1] {
								copied = copied || done == 0
							}
							if (out == 0x12345678) != copied {
								t.Fatal("CPU bytes do not match delivered copy response")
							}
						}
					}
				}
				if f.r.Unfinished()[0] != 0 || len(f.c.State.Executing) != 0 || len(f.snapshot().Queues[f.q-1].Pending) != 0 {
					t.Fatal("last reply did not release the copy and next gpuruntime.Noop")
				}
				if read {
					var out uint32
					if err := f.r.ReadHost(host, &out); err != nil {
						t.Fatal(err)
					}
					if out != 0x12345678 {
						t.Fatalf("result %x", out)
					}
				}
			})
		}
	}
}
func TestKernelSubmitAndCompletion(t *testing.T) {
	f := newFixture(t, false)
	code := insts.CodeObjectValue{Present: true, HasMeta: true, Version: insts.CodeObjectV5,
		Meta: insts.KernelCodeObjectMeta{WFSgprCount: 17}, Data: []byte{1, 2, 3, 4}}
	cmd := gpuruntime.Command{
		Kind: gpuruntime.LaunchKernel,
		Code: code,
		GPUs: []int{1},
		Packets: []kernels.PacketValue{{Packet: kernels.HsaKernelDispatchPacket{
			GridSizeX:      256,
			GridSizeY:      1,
			GridSizeZ:      1,
			WorkgroupSizeX: 64,
			WorkgroupSizeY: 1,
			WorkgroupSizeZ: 1,
		}}},
		PacketAddresses: []gpuruntime.Ptr{0},
	}
	f.enqueue(f.q, cmd)
	tickDriver(f.c)
	if len(f.c.State.Executing) != 1 || len(f.c.State.Executing[0].Requests) != 1 || len(f.c.State.RequestsToSend) != 1 {
		t.Fatal("kernel not submitted")
	}
	req := f.c.State.Executing[0].Requests[0]
	if req.Payload.(protocol.LaunchKernelReq).PID != 1 {
		t.Fatal("kernel lost PID")
	}
	payload := req.Payload.(protocol.LaunchKernelReq)
	if !payload.CodeObject.Present || !payload.CodeObject.HasMeta ||
		payload.CodeObject.Version != insts.CodeObjectV5 || payload.CodeObject.Meta.WFSgprCount != 17 {
		t.Fatal("launch lost code metadata")
	}
	if !bytes.Equal(f.c.State.Executing[0].Command.Code.Data, code.Data) {
		t.Fatal("launch modified command executable")
	}
	f.reply(req, protocol.LaunchKernelRsp{})
	if len(f.c.State.Executing) != 0 || f.r.Unfinished()[0] != 0 {
		t.Fatal("kernel did not complete")
	}
}
