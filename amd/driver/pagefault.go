package driver

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	fault "github.com/sarchlab/akita/v5/mem/vm/pagefaultprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

// Keep unanswered requests in arrival order. Each still owns an entry at
// the IOMMU, which bounds this list. A fresh fault waiting for space in the
// service queue must not prevent an active page's Hold from draining.
func (m *migrationMW) receivePageFaults() bool {
	c := m.comp
	s := &c.State.PageTables
	port := c.Ports.PageFault[0]
	progress := false
	for {
		msg, ok := port.RetrieveIncoming()
		if !ok {
			break
		}
		s.Faults = append(s.Faults, msg)
		progress = true
	}
	left := s.Faults[:0]
	for _, msg := range s.Faults {
		if !port.CanSend() {
			left = append(left, msg)
			continue
		}
		req := msg.Payload.(fault.Req)
		rsp, accepted := m.decide(req)
		if !accepted {
			left = append(left, msg)
			m.state().Stats.QueueFullWaits++
			continue
		}
		port.Send(messaging.Msg{ID: c.NewID(), RspTo: msg.ID, Src: port.AsRemote(), Dst: msg.Src, Payload: rsp})
		progress = true
	}
	clear(s.Faults[len(left):])
	s.Faults = left
	return progress
}

func (m *migrationMW) decide(req fault.Req) (fault.Rsp, bool) {
	va := req.VAddr &^ (m.pageSize() - 1)
	pi, ok := m.findPage(req.PID, va)
	active := ok && m.state().Pages[pi].Phase != pageIdle
	if !active && (!req.HasMapping || !req.Mapping.Valid) {
		panic(fmt.Sprintf("driver: unmapped fault PID %d VA %#x requested by GPU %d", req.PID, req.VAddr, req.DeviceID))
	}
	page := req.Mapping
	if !active && (m.spec().Policy != PageMigrationFault || !page.Unified || page.IsPinned ||
		page.DeviceID == req.DeviceID || !m.isGPU(int(page.DeviceID))) {
		return fault.Rsp{Result: fault.Resolved, Mapping: page}, true
	}
	accepted := m.acceptFault(addrtransprotocol.PageFault{PID: req.PID, VAddr: va, DeviceID: req.DeviceID})
	return fault.Rsp{Result: fault.Blocked}, accepted
}

func (m *migrationMW) startPin(pi int, page vm.Page) {
	p := &m.state().Pages[pi]
	p.Service = servicePin
	p.OldPage = page
	p.Waiters = nil
	p.ServiceStart = m.now()
	p.Targets = m.allGPUs()
	m.startHold(pi)
}

// commitMapping runs under the all-GPU Hold after copy or pin maintenance.
// The new physical page can become visible only after every MMU responds.
func (m *migrationMW) commitMapping(pi int, before vm.Page) {
	if len(m.comp.Spec.PageTables) == 0 {
		m.startShootdown(pi)
		return
	}
	p := &m.state().Pages[pi]
	after, found := m.comp.Resources.PageTable.Find(p.PID, p.VAddr)
	if !found && p.Service == serviceUnpin {
		m.startShootdown(pi)
		return
	}
	if !found {
		panic("driver: migration lost its allocation")
	}
	p.Phase = pageMapping
	m.comp.Middlewares.PageTable.update(before, after, pi+1)
}

func (m *migrationMW) mappingsReady(pi int) {
	if m.state().Pages[pi].Phase != pageMapping {
		panic("driver: unexpected mapping completion")
	}
	m.startShootdown(pi)
}
