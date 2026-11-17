package driver

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	fault "github.com/sarchlab/akita/v5/mem/vm/pagefaultprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// A new remote page cannot enter a full service queue. The IOMMU still
// needs an immediate decision for the next request, whose page is draining
// its Hold. A third, non-migratable request is independently usable.
func TestAnActiveFaultBypassesAFullServiceQueue(t *testing.T) {
	f := newRuntimeConfiguredFixture(t, false, func(s *Spec) {
		s.PageMigration.QueueSize = 1
		for i := range 3 {
			s.PageTables = append(s.PageTables, PageTableTarget{Control: messaging.RemotePort(fmt.Sprintf("MMU%d.Control", i)),
				PoolBase: uint64(i) << 32, PoolNodes: 1})
		}
	}, func(s *runtime.RuntimeSpec) { s.CPUPageTableBytes, s.GPUPageTableBytes = 4096, 4096 })
	m := f.c.Middlewares.Migration
	active := vm.Page{PID: 1, VAddr: 4096, PAddr: 1<<32 | 4096, DeviceID: 1, PageSize: 4096, Valid: true, Unified: true}
	newPage := active
	newPage.VAddr += 4096
	resolved := newPage
	resolved.VAddr += 4096
	resolved.Unified = false
	idx := m.pageIndex(active.PID, active.VAddr)
	m.state().Pages[idx].Phase = pageHold
	m.state().Queue = append(m.state().Queue, migrationRequest{Counter: true})
	port := f.c.Ports.PageFault[0]
	for i, p := range []vm.Page{newPage, active, resolved, newPage} {
		port.Deliver(messaging.Msg{ID: uint64(i + 1), Src: "IOMMU.PageFault", Dst: port.AsRemote(),
			Payload: fault.Req{PID: p.PID, VAddr: p.VAddr, HasMapping: true, Mapping: p, DeviceID: 2}})
	}
	m.receiveRequests()
	for i, want := range []fault.Result{fault.Blocked, fault.Resolved} {
		msg, ok := port.RetrieveOutgoing()
		if !ok || msg.RspTo != uint64(i+2) || msg.Payload.(fault.Rsp).Result != want {
			t.Fatalf("wrong decision %d: %+v", i, msg)
		}
	}
	if pending := m.state().Faults; len(pending) != 2 || pending[0].ID != 1 || pending[1].ID != 4 ||
		len(m.state().Queue) != 1 || m.state().Stats.QueueFullWaits != 1 {
		t.Fatalf("unanswered requests %v, queue %+v", pending, m.state().Queue)
	}
	m.state().Queue = nil
	m.receiveRequests()
	for _, id := range []uint64{1, 4} {
		msg, ok := port.RetrieveOutgoing()
		if !ok || msg.RspTo != id || msg.Payload.(fault.Rsp).Result != fault.Blocked || len(m.state().Faults) != 0 {
			t.Fatalf("deferred request %d did not receive its own decision: %+v", id, msg)
		}
	}
}
