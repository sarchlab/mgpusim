package timingconfig

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	fault "github.com/sarchlab/akita/v5/mem/vm/pagefaultprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/pagetablemmu"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type pteTraffic struct{ reads, writes int }

func (h *pteTraffic) Func(ctx hooking.HookCtx) {
	if ctx.Pos != messaging.HookPosPortMsgSend {
		return
	}
	switch ctx.Item.(messaging.Msg).Payload.(type) {
	case memprotocol.ReadReq:
		h.reads++
	case memprotocol.WriteReq:
		h.writes++
	}
}

func TestPageTablePlacementAndRemoteDecisions(t *testing.T) {
	for _, global := range []bool{false, true} {
		t.Run(fmt.Sprintf("global=%t", global), func(t *testing.T) {
			p := buildMigrationPlatform(t, "r9nano", func(b Builder) Builder {
				cfg := DefaultPageTableSpec()
				cfg.GlobalGPU = global
				cfg.GPU.FilterBuckets = 1
				// Routes belong to the platform, including range/fallback fields.
				cfg.IOMMU.Memory = mem.AddressMapper{
					Destinations: []messaging.RemotePort{"OtherMemory.Top"},
					RangeLow:     deviceBytes, RangeHigh: 2 * deviceBytes, Fallback: "OtherMemory.Fallback",
				}
				return b.WithPageTable(cfg)
			})
			if dst := p.mmu.Spec.Memory.Find(p.mmu.Spec.PoolBase); dst != cpuMemoryTop {
				t.Fatalf("IOMMU pool routes to %s, want %s", dst, cpuMemoryTop)
			}
			r := p.driver.Resources.Runtime
			ctx := r.Init()
			q := r.CreateCommandQueue(ctx)
			dev := r.AllocateMemory(ctx, 16)
			host := r.AllocateHostBuffer(q, 16)
			pid := r.QueuePID(q)
			peer := p.walkers["GPU[2].GMMU"]
			gpuTraffic, ioTraffic := &pteTraffic{}, &pteTraffic{}
			peer.Ports.Memory.AcceptHook(gpuTraffic)
			p.mmu.Ports.Memory.AcceptHook(ioTraffic)
			p.driver.TickLater()
			p.run(t)
			if ioTraffic.writes == 0 || (gpuTraffic.writes > 0) != global {
				t.Fatalf("PTE writes GPU %d IOMMU %d", gpuTraffic.writes, ioTraffic.writes)
			}
			for _, va := range []uint64{uint64(dev), uint64(host.Address)} {
				expected, _ := p.driver.Resources.PageTable.Find(pid, va)
				got := p.translate(t, peer.Ports.FaultIn, pid, va+1, 2)
				if got.Result != fault.Resolved || got.Mapping != expected {
					t.Fatalf("remote VA %#x: %+v, want %+v", va, got, expected)
				}
			}
			if ioTraffic.reads == 0 || (gpuTraffic.reads > 0) != global || p.driver.State.PageMigration.Stats.Faults != 0 {
				t.Fatalf("PTE traffic GPU %+v IOMMU %+v, migration %+v", gpuTraffic, ioTraffic, p.driver.State.PageMigration.Stats)
			}
		})
	}
}

func (p migrationPlatform) run(t *testing.T) {
	t.Helper()
	if err := p.sim.Run(); err != nil {
		t.Fatal(err)
	}
}

func (p migrationPlatform) translate(t *testing.T, dst messaging.Port, pid vm.PID, va, gpu uint64) fault.Rsp {
	t.Helper()
	id := p.sim.NewID()
	dst.Deliver(messaging.Msg{ID: id, Src: p.client.AsRemote(), Dst: dst.AsRemote(),
		Payload: fault.Req{PID: pid, VAddr: va, DeviceID: gpu}})
	p.run(t)
	msg, ok := p.client.RetrieveIncoming()
	if !ok || msg.RspTo != id {
		t.Fatalf("missing translation reply to %d", id)
	}
	return msg.Payload.(fault.Rsp)
}

func TestUnmappedFaultNamesPIDVAAndGPU(t *testing.T) {
	p := buildMigrationPlatform(t, "r9nano", func(b Builder) Builder {
		spec := driver.DefaultPageMigrationSpec()
		spec.Policy = driver.PageMigrationNone
		return b.WithPageMigration(spec)
	})
	p.client.Send(messaging.Msg{ID: p.sim.NewID(), Src: p.client.AsRemote(), Dst: p.mmu.Ports.FaultIn.AsRemote(),
		Payload: fault.Req{PID: 7, VAddr: 0x1234, DeviceID: 2}})
	const diagnostic = "unmapped fault PID 7 VA 0x1234 requested by GPU 2"
	if err := p.sim.Run(); err == nil || !strings.Contains(err.Error(), diagnostic) {
		t.Fatalf("terminal fault: %v", err)
	}
}

func TestSystemPageSizeAndFixedLatencyConfiguration(t *testing.T) {
	for _, fixed := range []bool{false, true} {
		t.Run(fmt.Sprintf("fixed=%t", fixed), func(t *testing.T) {
			p := buildMigrationPlatform(t, "mi300x", func(b Builder) Builder {
				spec := DefaultPageTableSpec()
				spec.FixedLatency = fixed
				spec.Log2PageSize = 16
				return b.WithPageTable(spec)
			})
			if (p.mmu == nil) != fixed || p.driver.Resources.PageTable.Log2PageSize() != 16 {
				t.Fatal("system selected the wrong walker or allocation page size")
			}
			for _, c := range p.sim.Components() {
				if bits, ok := componentPageBits(c); ok && bits != 16 {
					t.Fatalf("%s uses page bits %d", c.Name(), bits)
				}
			}
		})
	}
}

func componentPageBits(c any) (uint64, bool) {
	switch c := c.(type) {
	case *pagetablemmu.Comp:
		return uint64(c.Spec.Log2PageSize), true
	case *tlb.Comp:
		return c.Spec.Log2PageSize, true
	case *addresstranslator.Comp:
		return c.Spec.Log2PageSize, true
	case *mmu.Comp:
		return c.Spec.Log2PageSize, true
	default:
		return 0, false
	}
}
