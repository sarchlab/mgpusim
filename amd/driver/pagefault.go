package driver

import (
	"fmt"
	"slices"

	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	fault "github.com/sarchlab/akita/v5/mem/vm/pagefaultprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

// Keep unanswered requests in arrival order. Each still owns an entry at
// the IOMMU, which bounds this list. A fresh fault waiting for space in the
// service queue must not prevent an active page's Hold from draining.
func (m *migrationMW) receivePageFaults() (progress, queueWait bool) {
	c := m.comp
	s := m.state()
	port := c.Ports.PageFault[0]
	for {
		msg, ok := port.RetrieveIncoming()
		if !ok {
			break
		}
		s.Faults = append(s.Faults, msg)
		if t := c.TaskTracker(); t != nil {
			c.State.RequestTasks[msg.ID] = t.StartReqIn(c, msg)
		}
		progress = true
	}
	left := s.Faults[:0]
	for _, msg := range s.Faults {
		if !port.CanSend() {
			left = append(left, msg)
			continue
		}
		req := msg.Payload.(fault.Req)
		rsp, accepted := m.decide(req, c.State.RequestTasks[msg.ID])
		if !accepted {
			left = append(left, msg)
			queueWait = true
			continue
		}
		port.Send(messaging.Msg{ID: c.NewID(), RspTo: msg.ID, Src: port.AsRemote(), Dst: msg.Src, Payload: rsp})
		if rsp.Result == fault.Resolved && req.DeviceID != rsp.Mapping.DeviceID && m.isGPU(int(req.DeviceID)) {
			c.Middlewares.PageTable.fillRemote(rsp.Mapping, int(req.DeviceID), c.State.RequestTasks[msg.ID])
		}
		c.TaskTracker().EndReqIn(c.State.RequestTasks[msg.ID])
		delete(c.State.RequestTasks, msg.ID)
		progress = true
	}
	clear(s.Faults[len(left):])
	s.Faults = left
	return progress, queueWait
}

func (m *migrationMW) decide(req fault.Req, parent uint64) (fault.Rsp, bool) {
	va := req.VAddr &^ (m.pageSize() - 1)
	pi, ok := m.findPage(req.PID, va)
	active := ok && m.state().Pages[pi].Phase != pageIdle
	page, mapped := m.comp.Resources.PageTable.Find(req.PID, va)
	if !active && !mapped {
		panic(fmt.Sprintf("driver: unmapped fault PID %d VA %#x requested by GPU %d", req.PID, req.VAddr, req.DeviceID))
	}
	view := m.comp.State.PageTables.Views[req.PID][va]
	if !active && (m.spec().Policy != PageMigrationFault || !page.Unified || page.IsPinned ||
		slices.Contains(view.Explicit, int(req.DeviceID)) || page.DeviceID == req.DeviceID || !m.isGPU(int(page.DeviceID))) {
		return fault.Rsp{Result: fault.Resolved, Mapping: page}, true
	}
	accepted := m.acceptFault(addrtransprotocol.PageFault{PID: req.PID, VAddr: va, DeviceID: req.DeviceID}, parent)
	return fault.Rsp{Result: fault.Blocked}, accepted
}
