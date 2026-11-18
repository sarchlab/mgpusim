package driver

import (
	"fmt"
	"slices"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	fault "github.com/sarchlab/akita/v5/mem/vm/pagefaultprotocol"
	pt "github.com/sarchlab/akita/v5/mem/vm/pagetableprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/runtime"
)

func newTableFixture(t *testing.T) *fixture {
	t.Helper()
	return newRuntimeConfiguredFixture(t, false, func(s *Spec) {
		for i := range 3 {
			s.PageTables = append(s.PageTables, PageTableTarget{Control: messaging.RemotePort(fmt.Sprintf("MMU%d.Control", i)),
				PoolBase: uint64(i) << 32, PoolNodes: 16})
		}
	}, func(s *runtime.RuntimeSpec) { s.CPUPageTableBytes, s.GPUPageTableBytes = 65536, 65536 })
}

func (f *fixture) answerTables(t *testing.T) {
	t.Helper()
	table := f.c.Middlewares.PageTable
	port := f.c.Ports.PageTable[0]
	for table.Handle(nil) {
		for {
			req, ok := port.RetrieveOutgoing()
			if !ok {
				break
			}
			port.Deliver(messaging.Msg{ID: f.s.NewID(), Src: req.Dst, Dst: req.Src,
				RspTo: req.ID, Payload: pt.Rsp{}})
		}
	}
}

func TestAccessedByFollowsHostJournalOrder(t *testing.T) {
	f := newTableFixture(t)
	pid := f.r.QueuePID(f.q)
	other := f.r.Init()
	otherPID := f.r.QueuePID(f.r.CreateCommandQueue(other))
	addr := f.r.AllocateMemory(f.ctx, 3*testPageSize)
	if f.r.AllocateMemory(other, 3*testPageSize) != addr {
		t.Fatal("expected the same virtual addresses in two PIDs")
	}
	f.r.SetAccessedBy(f.r.InitWithExistingPID(f.ctx), addr+testPageSize-1, 2, 2)
	f.r.Remap(f.ctx, uint64(addr), testPageSize, 1)
	dropped := f.r.AllocateMemory(f.ctx, 1)
	f.r.SetAccessedBy(f.ctx, dropped, 1, 2)
	if err := f.r.FreeMemory(f.ctx, dropped); err != nil {
		t.Fatal(err)
	}
	f.c.Middlewares.PageTable.hostChanges()
	var maps []vm.Page
	for _, op := range f.c.State.PageTables.Streams[2].Queue {
		if op.Request.Command == pt.Map {
			maps = append(maps, op.Request.Mapping)
		}
	}
	first, _ := f.p.Find(pid, uint64(addr))
	second, _ := f.p.Find(pid, uint64(addr)+testPageSize)
	otherView := f.c.State.PageTables.Views[otherPID][uint64(addr)]
	if !slices.Equal(maps, []vm.Page{first, second}) || otherView.Explicit != nil {
		t.Fatalf("unexpected remote publications %+v", maps)
	}
}

func (f *fixture) requestDecision(t *testing.T, page vm.Page, gpu uint64) fault.Rsp {
	t.Helper()
	port := f.c.Ports.PageFault[0]
	req := messaging.Msg{ID: f.s.NewID(), Src: "IOMMU.PageFault", Dst: port.AsRemote(),
		Payload: fault.Req{PID: page.PID, VAddr: page.VAddr, HasMapping: true, Mapping: page, DeviceID: gpu}}
	port.Deliver(req)
	f.c.Middlewares.Migration.Tick()
	rsp, ok := port.RetrieveOutgoing()
	if !ok || rsp.RspTo != req.ID {
		t.Fatalf("missing decision for %+v", req)
	}
	return rsp.Payload.(fault.Rsp)
}

func TestLazyMapPromotionAndExpiry(t *testing.T) {
	f := newTableFixture(t)
	m := f.c.Middlewares.Migration
	table := f.c.Middlewares.PageTable
	va := f.r.AllocateUnifiedMemory(f.ctx, testPageSize)
	f.answerTables(t)
	page, _ := f.p.Find(f.r.QueuePID(f.q), uint64(va))
	pi := m.pageIndex(page.PID, page.VAddr)
	m.pin(pi, page)
	pinned, _ := f.p.Find(page.PID, page.VAddr)
	table.publish(mappingGroup{PID: page.PID, VAddr: page.VAddr}, pageView{Page: pinned}, 0)
	f.answerTables(t)
	// Leave the lazy Map outstanding. The first walk must retire immediately
	// using the current pinned mapping even when the IOMMU saw the old PTE.
	if got := f.requestDecision(t, page, 2); got.Result != fault.Resolved || got.Mapping != pinned {
		t.Fatalf("decision before any Map ACK: %+v", got)
	}
	stream := &f.c.State.PageTables.Streams[2]
	queueLen := len(stream.Queue)
	if queueLen == 0 || stream.Queue[queueLen-1].Request.Command != pt.Map {
		t.Fatalf("missing lazy Map: %+v", stream.Queue)
	}
	f.r.SetAccessedBy(f.ctx, va, 1, 2)
	table.hostChanges()
	if len(stream.Queue) != queueLen || f.c.State.PageTables.PendingPages[page.PID][page.VAddr] != 1 {
		t.Fatal("advice failed to join the pending Map")
	}
	m.state().Pages[pi].Service = serviceUnpin
	m.held(pi)
	if stream.Queue[queueLen].Request.Command != pt.Remap ||
		stream.Queue[queueLen].Request.Mapping.IsPinned || m.state().Pages[pi].Phase != pageMapping {
		t.Fatalf("explicit unpin did not follow the lazy Map: %+v", stream.Queue)
	}
	f.answerTables(t)
	if m.state().Pages[pi].Phase != pageShootdown {
		t.Fatal("expected shootdown only after all PTEs were acknowledged")
	}
}

func TestTemporaryRemoteMapRevokedBeforeRelease(t *testing.T) {
	f := newTableFixture(t)
	m := f.c.Middlewares.Migration
	c := f.c
	va := f.r.AllocateUnifiedMemory(f.ctx, testPageSize)
	f.answerTables(t)
	page, _ := f.p.Find(f.r.QueuePID(f.q), uint64(va))
	pi := m.pageIndex(page.PID, page.VAddr)
	m.pin(pi, page)
	pinned, _ := f.p.Find(page.PID, page.VAddr)
	c.Middlewares.PageTable.publish(mappingGroup{PID: page.PID, VAddr: page.VAddr}, pageView{Page: pinned}, 0)
	f.answerTables(t)
	f.requestDecision(t, pinned, 2)
	p := &m.state().Pages[pi]
	p.PinnedUntil = m.now()
	m.expirePins()
	if got := f.requestDecision(t, pinned, 2); got.Result != fault.Blocked || p.Phase != pageHold {
		t.Fatalf("fault during Hold: %+v, phase %d", got, p.Phase)
	}
	mf := migrationFixture{fixture: f, t: t}
	mf.step(page)
	q := c.State.PageTables.Streams[2].Queue
	if len(q) < 2 || q[len(q)-2].Request.Command != pt.Map || q[len(q)-1].Request.Command != pt.Unmap ||
		p.Phase != pageMapping || len(mf.of(page)) != 0 {
		t.Fatalf("unpin must wait for Map then Unmap before shootdown: %+v, phase %d", q, p.Phase)
	}
	f.answerTables(t)
	mf.ticks()
	mf.step(page)
	releases := mf.of(page)
	if len(releases) != 2 || p.Phase != pageRelease {
		t.Fatalf("expected two Release requests: %+v", releases)
	}
	mf.answer(releases[0])
	if p.Phase != pageRelease {
		t.Fatal("retained fault started before the final Release ACK")
	}
	mf.answer(releases[1])
	// The retained remote requester now enters migration policy.
	if p.Service != serviceMigrate || p.Phase != pageHold || p.OldPage != page {
		t.Fatalf("fault did not re-enter migration after unpin: %+v", p)
	}
}
