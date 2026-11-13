package pagemigration

import (
	"bytes"
	"fmt"
	"slices"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	writethroughcache "github.com/sarchlab/akita/v5/mem/cache/writethrough"
	"github.com/sarchlab/akita/v5/mem/datamover"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	directconnection "github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/gpubuilder"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp"
)

// The memory of a writebackPlatform: pages of 4 KiB and lines of 64 bytes.
// The L2 has room for two lines, so a third line evicts the one that was
// used least recently.
const (
	benchPageSize = 4096
	benchLine     = 64
	benchL2Lines  = 2
)

// What happens to the new frame of the page, as a writebackPlatform records
// it.
const (
	eventWriteback  = "the L2 writes a line back"
	eventInvalidate = "the L2 has dropped the lines of the frame"
	eventCopy       = "the data mover starts the copy"
)

// bench is what the test plays in a writebackPlatform. It owns ports, which
// only receive, and does nothing by itself: the test delivers what the bench
// sends.
//
// L1 reads and writes the L2 of GPU 2, as the caches above it do. Between
// the L2 and the memory below it is a gate: what the L2 sends down arrives
// at GateTop and goes on from GateBottom once the test lets it pass, and the
// answers of the memory go back up. While the gate is closed, a line that
// the L2 writes back is on its way and is not acknowledged.
type bench struct {
	p *writebackPlatform

	l1, gateTop, gateBottom messaging.Port

	// l2Bottom and below are where the gate passes messages on to.
	l2Bottom, below messaging.Port

	open    bool
	waiting []messaging.Msg
}

func (b *bench) Name() string { return "Bench" }

func (b *bench) BindPort(slot string, port messaging.Port) {
	port.(*twowaybuffered.Port).BindOwner(b, b.Name()+"."+slot)
}

func (b *bench) NotifyRecv(messaging.Port)     {}
func (b *bench) NotifyPortFree(messaging.Port) {}

// pump takes what the L2 has sent down, passes it on if the gate is open,
// and passes the answers of the memory up. It reports whether it passed
// anything on.
func (b *bench) pump() bool {
	for {
		msg, ok := b.gateTop.RetrieveIncoming()
		if !ok {
			break
		}

		b.waiting = append(b.waiting, msg)
	}

	passed := false

	if b.open {
		for _, msg := range b.waiting {
			if _, ok := msg.Payload.(memprotocol.WriteReq); ok {
				b.p.events = append(b.p.events, eventWriteback)
			}

			msg.Src, msg.Dst = b.gateBottom.AsRemote(), b.below.AsRemote()
			b.below.Deliver(msg)

			passed = true
		}

		b.waiting = nil
	}

	for {
		msg, ok := b.gateBottom.RetrieveIncoming()
		if !ok {
			return passed
		}

		msg.Src, msg.Dst = b.gateTop.AsRemote(), b.l2Bottom.AsRemote()
		b.l2Bottom.Deliver(msg)

		passed = true
	}
}

// holdsWrite reports whether a write of data to an address waits at the gate.
func (b *bench) holdsWrite(addr uint64, data []byte) bool {
	return slices.ContainsFunc(b.waiting, func(msg messaging.Msg) bool {
		write, ok := msg.Payload.(memprotocol.WriteReq)
		return ok && write.Address == addr && bytes.Equal(write.Data, data)
	})
}

// portHook calls record with the messages that pass a hook position of a
// port.
type portHook struct {
	pos    *hooking.HookPos
	record func(messaging.Msg)
}

func (h *portHook) Func(ctx hooking.HookCtx) {
	if ctx.Pos == h.pos {
		h.record(ctx.Item.(messaging.Msg))
	}
}

// writebackPlatform is the part of a platform of two GPUs that takes a page
// from GPU 1 to GPU 2, built from the components of the platforms: the
// driver, the command processors, the L2 cache and the data mover of GPU 2,
// and the memory of both GPUs. GPU 1 has no L2, and no GPU has address
// translators or TLBs, so those steps of a migration are answered at once.
type writebackPlatform struct {
	t       *testing.T
	sim     *sim.Simulation
	engine  *timing.SerialEngine
	wires   *directconnection.Connection
	storage *mem.Storage
	table   *vm.PageTable
	rt      *gpuruntime.Runtime
	ctx     gpuruntime.Context
	pid     vm.PID

	driver *driver.Comp
	cps    [2]*cp.Comp
	l2     *writeback.Comp
	mover  *datamover.Comp
	bench  *bench

	// events are what happened to the new frame of the page, in order.
	events []string
}

// belowL2 builds the memory below the L2 of GPU 2 and returns its Top port.
type belowL2 func(p *writebackPlatform) messaging.Port

// dramBelowL2 puts the DRAM below the L2, as on R9 Nano.
func dramBelowL2(p *writebackPlatform) messaging.Port {
	return p.dram("GPU[2].DRAM").Ports.Top
}

// mallBelowL2 puts the MALL, a write-through cache in front of the DRAM,
// below the L2, as on MI300X.
func mallBelowL2(p *writebackPlatform) messaging.Port {
	spec := writethroughcache.DefaultSpec()
	spec.WritePolicy = writethroughcache.WritePolicyWriteThrough
	spec.NumSets = benchPageSize / benchLine
	spec.Bottom = mem.AddressMapper{
		Destinations: []messaging.RemotePort{p.dram("GPU[2].DRAM").Ports.Top.AsRemote()},
	}

	mall := modeling.MakeBuilder[*writethroughcache.Comp](spec).WithSimulation(p.sim).
		WithResources(writethroughcache.Resources{Storage: p.cacheStorage("GPU[2].MALL", spec.ByteSize())}).
		Build("GPU[2].MALL")
	p.ports(mall, "Top", "Bottom", "Control")

	return mall.Ports.Top
}

func (p *writebackPlatform) cacheStorage(name string, bytes uint64) *mem.Storage {
	return mem.MakeStorageBuilder().WithSimulation(p.sim).WithCapacity(bytes).Build(name + ".Storage")
}

// ports builds the ports of a component and connects them. One connection
// carries everything: the ports find each other by name.
func (p *writebackPlatform) ports(owner interface {
	BindPort(string, messaging.Port)
}, slots ...string) []messaging.Port {
	var built []messaging.Port

	for _, slot := range slots {
		port := twowaybuffered.MakeBuilder().WithSimulation(p.sim).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 1024, OutgoingCapacity: 1024}).
			Build()
		owner.BindPort(slot, port)
		p.wires.BindPort(port)

		built = append(built, port)
	}

	return built
}

// dram builds a memory controller on the global storage.
func (p *writebackPlatform) dram(name string) *idealmemcontroller.Comp {
	spec := idealmemcontroller.DefaultSpec()
	spec.NumReqPerCycle = 16

	dram := modeling.MakeBuilder[*idealmemcontroller.Comp](spec).WithSimulation(p.sim).
		WithResources(idealmemcontroller.Resources{Storage: p.storage}).
		Build(name)
	p.ports(dram, "Top", "Control")

	return dram
}

func (p *writebackPlatform) commandProcessor(name string, change func(*cp.Spec)) *cp.Comp {
	spec := cp.DefaultSpec()
	spec.Driver = "Driver." + driver.GPUPortName
	change(&spec)

	c := cp.MakeBuilder().WithSimulation(p.sim).WithSpec(spec).Build(name)
	p.ports(c, "ToDriver", "ToDMA", "ToCUs", "ToCaches", "ToRDMA",
		"ToTranslation", "ToDataMover", "PageMigration")

	return c
}

//nolint:funlen // Keep the wiring of the platform in one place.
func newWritebackPlatform(t *testing.T, memory belowL2) *writebackPlatform {
	t.Helper()

	if testing.Short() {
		t.Skip("page-migration acceptance builds and runs simulations; run without -short")
	}

	p := &writebackPlatform{t: t, sim: sim.MakeBuilder().Build(), table: vm.NewPageTable(12)}
	t.Cleanup(func() {
		if err := p.sim.Terminate(); err != nil {
			t.Error(err)
		}
	})

	p.engine = p.sim.Engine().(*timing.SerialEngine)
	p.wires = directconnection.NewConnection("Wires", p.sim, timing.GHz)

	// The CPU and both GPUs have 4 GiB, as on the platforms, and a freed
	// frame is the next one that is handed out.
	runtimeSpec := gpuruntime.DefaultRuntimeSpec()
	runtimeSpec.Log2PageSize = 12
	runtimeSpec.Allocator = gpuruntime.AllocatorBuddy
	runtimeSpec.Devices = []gpuruntime.DeviceProperties{
		{CUCount: 1, DRAMSize: 4 * mem.GB}, {CUCount: 1, DRAMSize: 4 * mem.GB},
	}

	p.storage = mem.MakeStorageBuilder().WithSimulation(p.sim).WithCapacity(12 * mem.GB).Build("GlobalStorage")
	p.rt = gpuruntime.NewRuntime(p.sim, "Runtime", runtimeSpec, p.table, p.storage)

	// The memory of both GPUs, and the gate above the memory of GPU 2.
	source := p.dram("GPU[1].DRAM").Ports.Top.AsRemote()
	below := memory(p)
	destination := below.AsRemote()

	p.bench = &bench{p: p, open: true, below: below}
	benchPorts := p.ports(p.bench, "L1", "GateTop", "GateBottom")
	p.bench.l1, p.bench.gateTop, p.bench.gateBottom = benchPorts[0], benchPorts[1], benchPorts[2]

	// The L2 of GPU 2 reaches the memory below it through the gate.
	l2Spec := writeback.DefaultSpec()
	l2Spec.NumSets = 1
	l2Spec.NumWays = benchL2Lines
	l2Spec.Bottom = mem.AddressMapper{Destinations: []messaging.RemotePort{p.bench.gateTop.AsRemote()}}
	p.l2 = modeling.MakeBuilder[*writeback.Comp](l2Spec).WithSimulation(p.sim).
		WithResources(writeback.Resources{Storage: p.cacheStorage("GPU[2].L2Cache", l2Spec.ByteSize())}).
		Build("GPU[2].L2Cache")
	p.ports(p.l2, "Top", "Bottom", "Control")
	p.bench.l2Bottom = p.l2.Ports.Bottom

	// The data mover of GPU 2 copies around the L2, and so around the gate.
	p.mover = gpubuilder.BuildDataMover(p.sim, "GPU[2]", timing.GHz, benchLine,
		mem.AddressMapper{Destinations: []messaging.RemotePort{destination}},
		gpubuilder.PageMigrationConfig{
			DataMoverPages: 1,
			PeerMemory:     mem.AddressMapper{Destinations: []messaging.RemotePort{source}},
		})
	for _, port := range []messaging.Port{
		p.mover.Ports.Top, p.mover.Ports.Control, p.mover.Ports.Inside, p.mover.Ports.Outside,
	} {
		p.wires.BindPort(port)
	}

	p.cps[0] = p.commandProcessor("GPU[1].CommandProcessor", func(*cp.Spec) {})
	p.cps[1] = p.commandProcessor("GPU[2].CommandProcessor", func(s *cp.Spec) {
		s.L2Caches = []messaging.RemotePort{p.l2.Ports.Control.AsRemote()}
		s.DataMover = p.mover.Ports.Top.AsRemote()
	})

	spec := driver.DefaultSpec()
	spec.PageMigration.PrefetchThreshold = 0

	for _, c := range p.cps {
		spec.GPUs = append(spec.GPUs, c.Ports.ToDriver.AsRemote())
		spec.MigrationAgents = append(spec.MigrationAgents, c.Ports.PageMigration.AsRemote())
	}

	p.driver = driver.MakeBuilder().WithSimulation(p.sim).WithSpec(spec).
		WithResources(driver.Resources{Runtime: p.rt, GlobalStorage: p.storage, PageTable: p.table}).
		Build("Driver")
	p.ports(p.driver, driver.GPUPortName, driver.PageMigrationPortName, driver.MigrationNotificationPortName)

	p.recordEvents()

	if err := p.sim.Initialize(); err != nil {
		t.Fatal(err)
	}

	p.ctx = p.rt.Init()
	p.pid = p.rt.QueuePID(p.rt.CreateCommandQueue(p.ctx))

	return p
}

// recordEvents records when the L2 of GPU 2 acknowledges an Invalidate and
// when the data mover gets a copy. The gate records the writebacks that it
// lets pass.
func (p *writebackPlatform) recordEvents() {
	p.l2.Ports.Control.AcceptHook(&portHook{
		pos: messaging.HookPosPortMsgSend,
		record: func(msg messaging.Msg) {
			if rsp, ok := msg.Payload.(memcontrolprotocol.Rsp); ok && rsp.Command == memcontrolprotocol.CmdInvalidate {
				p.events = append(p.events, eventInvalidate)
			}
		},
	})

	p.mover.Ports.Top.AcceptHook(&portHook{
		pos: messaging.HookPosPortMsgRecvd,
		record: func(msg messaging.Msg) {
			if _, ok := msg.Payload.(datamoverprotocol.Req); ok {
				p.events = append(p.events, eventCopy)
			}
		},
	})
}

// settle runs the simulation until nothing moves anymore.
func (p *writebackPlatform) settle() {
	p.t.Helper()

	for {
		if err := p.engine.Run(); err != nil {
			p.t.Fatal(err)
		}

		if !p.bench.pump() {
			return
		}
	}
}

func (p *writebackPlatform) page(ptr gpuruntime.Ptr) vm.Page {
	p.t.Helper()

	page, ok := p.table.Find(p.pid, uint64(ptr))
	if !ok {
		p.t.Fatalf("no page at %#x", ptr)
	}

	return page
}

// access delivers a request of a cache above the L2 of GPU 2 to the L2.
func (p *writebackPlatform) access(payload any) {
	top := p.l2.Ports.Top

	top.Deliver(messaging.Msg{
		ID: p.sim.NewID(), Src: p.bench.l1.AsRemote(), Dst: top.AsRemote(), Payload: payload,
	})
}

// write writes a line of the L2 of GPU 2 as a cache above it does.
func (p *writebackPlatform) write(addr uint64, data []byte) {
	dirty := make([]bool, len(data))
	for i := range dirty {
		dirty[i] = true
	}

	p.access(memprotocol.WriteReq{Address: addr, Data: data, DirtyMask: dirty})
}

// read reads a line through the L2 of GPU 2 as a cache above it does.
func (p *writebackPlatform) read(addr uint64) []byte {
	p.t.Helper()

	p.access(memprotocol.ReadReq{Address: addr, AccessByteSize: benchLine})
	p.settle()

	for {
		msg, ok := p.bench.l1.RetrieveIncoming()
		if !ok {
			p.t.Fatalf("the L2 did not answer the read of %#x", addr)
		}

		if rsp, ok := msg.Payload.(memprotocol.DataReadyRsp); ok {
			return rsp.Data
		}
	}
}

// fault reports the demand fault of a GPU on a page to the driver, as the
// MMU does.
func (p *writebackPlatform) fault(ptr gpuruntime.Ptr, gpu uint64) {
	page := p.page(ptr)
	port := p.driver.Ports.MigrationNotification

	port.Deliver(messaging.Msg{
		ID:  p.sim.NewID(),
		Src: "MMU.Notification[0]",
		Dst: port.AsRemote(),
		Payload: addrtransprotocol.PageFault{
			PID: page.PID, VAddr: page.VAddr, DeviceID: gpu, Page: page,
		},
	})
}

// mustWaitForTheWriteback checks that the migration of a page to a frame of
// GPU 2 has come to rest before the copy: the destination has been asked to
// drop the L2 lines of the frame and has not answered.
func (p *writebackPlatform) mustWaitForTheWriteback(shared gpuruntime.Ptr, frame uint64) {
	p.t.Helper()

	if got := p.driver.State.PageMigration.Pages[0].NewPAddr; got != frame {
		p.t.Fatalf("the page moves to frame %#x, want the freed frame %#x", got, frame)
	}

	var jobs []any
	for _, job := range p.cps[1].State.PageJobs {
		jobs = append(jobs, job.Req.Payload)
	}

	invalidate := protocol.PageInvalidateReq{PAddr: frame, PageSize: benchPageSize}
	if len(jobs) != 1 || jobs[0] != invalidate || len(p.driver.State.PageMigration.Pending) != 1 {
		p.t.Fatalf("the command processor of GPU 2 works on %+v, want only %+v", jobs, invalidate)
	}

	if len(p.events) != 0 || len(p.mover.State.Transactions) != 0 || len(p.cps[1].State.MoveJobs) != 0 {
		p.t.Fatalf("the copy did not wait for the writeback of the old line: %q, %d copies in the data mover",
			p.events, len(p.mover.State.Transactions))
	}

	if page := p.page(shared); page.DeviceID != 1 {
		p.t.Fatalf("the page is mapped to GPU %d before it is copied", page.DeviceID)
	}
}

// The frame that a page migrates to can have been used before, and the L2 of
// the destination can be writing a dirty line of it back when the migration
// starts: another line has taken its place, and the memory below the L2 has
// not acknowledged the write. If the page were copied now, that old line
// could arrive in memory after the copy and replace a line of the page. The
// destination therefore answers the Invalidate of the frame only when the
// writeback is acknowledged, and the driver starts the copy only with that
// answer. The page then has its data, in the memory below the L2 and through
// the L2, with the DRAM below the L2 as on R9 Nano and with the MALL below it
// as on MI300X. The command processors, the L2, the data mover, and the
// memories are the ones of the platforms; the test plays the caches above
// the L2 and holds the writeback back.
//
//nolint:funlen // Keep the old lines, the delayed writeback, and the migration together.
func TestTheCopyWaitsForTheWritebackOfAnOldLineOfTheNewFrame(t *testing.T) {
	for _, platform := range []struct {
		name  string
		below belowL2
	}{
		{"r9nano", dramBelowL2},
		{"mi300x", mallBelowL2},
	} {
		t.Run(platform.name, func(t *testing.T) {
			p := newWritebackPlatform(t, platform.below)

			// A unified page with data, in the memory of GPU 1.
			shared := p.rt.AllocateUnifiedMemory(p.ctx, benchPageSize)
			data := make([]byte, benchPageSize)

			for i := range data {
				data[i] = byte(1 + i%251)
			}

			p.storage.Write(p.page(shared).PAddr, data)

			// GPU 2 uses a page of its own. Two of its lines are dirty in the
			// L2 when the page is freed: the memory does not have them.
			p.rt.SelectGPU(p.ctx, 2)
			old := p.rt.AllocateMemory(p.ctx, benchPageSize)
			frame := p.page(old).PAddr
			evicted, kept := frame, frame+benchLine
			junk := bytes.Repeat([]byte{0xee}, benchLine)

			p.write(evicted, junk)
			p.write(kept, junk)
			p.settle()

			if !bytes.Equal(p.storage.Read(frame, 2*benchLine), make([]byte, 2*benchLine)) {
				t.Fatal("the old lines are in memory already; the test needs dirty L2 lines")
			}

			if err := p.rt.FreeMemory(p.ctx, old); err != nil {
				t.Fatal(err)
			}

			// A line of another page takes the place of the first old line,
			// so the L2 writes the old line back. The write does not get
			// through: it is on its way, and not acknowledged.
			p.bench.open = false
			p.write(frame+benchPageSize, junk)
			p.settle()

			if !p.bench.holdsWrite(evicted, junk) {
				t.Fatal("the L2 is not writing the old line back; the test needs a writeback on its way")
			}

			// GPU 2 faults on the page of GPU 1, and the page gets the freed
			// frame. The migration comes to rest before the copy.
			p.fault(shared, 2)
			p.settle()
			p.mustWaitForTheWriteback(shared, frame)

			// The writeback gets through and is acknowledged: the destination
			// answers, and only then is the page copied.
			p.bench.open = true
			p.settle()

			if want := []string{eventWriteback, eventInvalidate, eventCopy}; !slices.Equal(p.events, want) {
				t.Fatalf("events %q, want %q", p.events, want)
			}

			moved := p.page(shared)
			if moved.DeviceID != 2 || moved.PAddr != frame || p.driver.State.PageMigration.Stats.PagesMigrated != 1 {
				t.Fatalf("the page did not migrate to the freed frame: %+v", moved)
			}

			// The page has its data below the L2, where the old line that
			// was written back is gone, and through the L2, which has
			// dropped the old line that it kept.
			if got := p.storage.Read(frame, benchPageSize); !bytes.Equal(got, data) {
				t.Fatalf("the memory below the L2 does not have the page: %s", firstDifference(got, data))
			}

			for _, line := range []uint64{evicted, kept} {
				offset := line - frame
				if got := p.read(line); !bytes.Equal(got, data[offset:offset+benchLine]) {
					t.Fatalf("the L2 reads line %#x of the page as % x...", offset, got[:8])
				}
			}
		})
	}
}

// firstDifference tells where two byte slices differ first.
func firstDifference(got, want []byte) string {
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			return fmt.Sprintf("byte %#x is %#x, want %#x", i, got[min(i, len(got)-1)], want[i])
		}
	}

	return "no difference"
}
