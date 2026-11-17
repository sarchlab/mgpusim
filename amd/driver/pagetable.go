package driver

import (
	"fmt"
	"slices"

	"github.com/sarchlab/akita/v5/mem/vm"
	pt "github.com/sarchlab/akita/v5/mem/vm/pagetableprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	rt "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// PageTableTarget identifies one MMU and its reserved 4 KiB node pool.
type PageTableTarget struct {
	Control   messaging.RemotePort `json:"control"`
	PoolBase  uint64               `json:"pool_base"`
	PoolNodes uint64               `json:"pool_nodes"`
}

type tableCommand struct {
	Request pt.Req
	Group   uint64
}

type tableStream struct {
	Roots  map[vm.PID]bool
	Queue  []tableCommand
	Flight uint64
}

type mappingGroup struct {
	PID     vm.PID
	VAddr   uint64
	Pending int
	// Migration is zero for host updates, or one plus the page's service index.
	Migration int
}

type pageTableState struct {
	Streams    []tableStream
	Groups     map[uint64]mappingGroup
	HostPages  map[vm.PID]map[uint64]int
	Dispatched map[vm.PID]bool
}

func (s *pageTableState) initialize(c *Comp) {
	if len(c.Spec.PageTables) == 0 {
		return
	}
	if len(c.Spec.PageTables) != len(c.Spec.GPUs)+1 || len(c.Ports.PageTable) != 1 || len(c.Ports.PageFault) != 1 ||
		len(c.Spec.MigrationAgents) != len(c.Spec.GPUs) {
		panic("driver: detailed translation requires a CPU and each GPU target, fault/control ports and shootdown agents")
	}
	for i, target := range c.Spec.PageTables {
		base, bytes := c.Resources.Runtime.PageTablePool(i)
		if target.Control == "" || target.PoolNodes == 0 || target.PoolBase != base || target.PoolNodes != bytes/4096 {
			panic("driver: missing MMU control port or pool differs from the allocator reservation")
		}
		s.Streams = append(s.Streams, tableStream{Roots: make(map[vm.PID]bool)})
	}
	s.Groups = make(map[uint64]mappingGroup)
	s.HostPages = make(map[vm.PID]map[uint64]int)
	s.Dispatched = make(map[vm.PID]bool)
}

type pageTableMW struct{ c *Comp }

func (m *pageTableMW) Handle(_ timing.Event) bool {
	c := m.c
	if len(c.Spec.PageTables) == 0 {
		return false
	}
	progress := m.hostChanges()
	progress = m.receive() || progress
	port := c.Ports.PageTable[0]
	for i := range c.State.PageTables.Streams {
		stream := &c.State.PageTables.Streams[i]
		if stream.Flight != 0 || len(stream.Queue) == 0 || !port.CanSend() {
			continue
		}
		stream.Flight = c.NewID()
		port.Send(messaging.Msg{ID: stream.Flight, Src: port.AsRemote(), Dst: c.Spec.PageTables[i].Control,
			Payload: stream.Queue[0].Request})
		progress = true
	}
	return progress
}

// Coalesce changes within the host round before publishing them. For example,
// allocation followed by Distribute needs only the final physical placements.
func (m *pageTableMW) hostChanges() bool {
	changes := m.c.Resources.Runtime.TakeMappingChanges()
	if len(changes) == 0 {
		return false
	}
	type key struct {
		pid vm.PID
		va  uint64
	}
	index := make(map[key]int)
	merged := changes[:0]
	for _, change := range changes {
		page := change.After
		if !page.Valid {
			page = change.Before
		}
		k := key{page.PID, page.VAddr}
		if i, ok := index[k]; ok {
			merged[i].After = change.After
		} else {
			index[k] = len(merged)
			merged = append(merged, change)
		}
	}
	for _, change := range merged {
		if change.Before != change.After {
			m.update(change.Before, change.After, 0)
		}
	}
	return true
}

func (m *pageTableMW) ready(q rt.CommandQueue) bool {
	c := m.c
	if len(c.Spec.PageTables) == 0 {
		return true
	}
	r := c.Resources.Runtime
	pending := c.State.PageTables.HostPages[r.QueuePID(q)]
	if len(pending) == 0 {
		return true
	}
	cmd, ok := r.PeekCommand(q)
	if !ok {
		return true
	}
	switch cmd.Kind {
	case rt.LaunchKernel:
		// Kernel arguments may reference any allocation shared by this PID.
		return false
	case rt.CopyH2D, rt.CopyD2H:
		size := uint64(1) << c.Resources.PageTable.Log2PageSize()
		for _, addr := range []rt.Ptr{cmd.Src, cmd.Dst} {
			for p := uint64(addr) &^ (size - 1); p < uint64(addr)+uint64(cmd.ByteSize); p += size {
				if pending[p] > 0 {
					return false
				}
			}
		}
	}
	return true
}

func (m *pageTableMW) contains(target int, p vm.Page) bool {
	return p.Valid && (target == 0 || uint64(target) == p.DeviceID || m.c.Spec.GlobalGPUPageTable && p.DeviceID != 0)
}

// update enqueues commands independently at every affected MMU. Each stream
// waits for its previous ACK, including registration of the PID's root.
func (m *pageTableMW) update(before, after vm.Page, migration int) {
	c := m.c
	s := &c.State.PageTables
	p := after
	if !p.Valid {
		p = before
	}
	group := c.NewID()
	g := mappingGroup{PID: p.PID, VAddr: p.VAddr, Migration: migration}
	for i := range s.Streams {
		had, has := m.contains(i, before), m.contains(i, after)
		if !had && !has {
			continue
		}
		stream := &s.Streams[i]
		if !stream.Roots[p.PID] {
			target := c.Spec.PageTables[i]
			n := uint64(len(stream.Roots))
			if n >= target.PoolNodes {
				panic("driver: MMU root pool exhausted")
			}
			// Roots descend; Akita allocates intermediate nodes upward. With
			// no PID teardown the two fronts meet only when this pool is full.
			root := target.PoolBase + (target.PoolNodes-1-n)*4096
			stream.Queue = append(stream.Queue, tableCommand{Request: pt.Req{Command: pt.RegisterRoot, PID: p.PID, Root: root}})
			stream.Roots[p.PID] = true
		}
		command := pt.Map
		if had {
			command = pt.Remap
		}
		if !has {
			command = pt.Unmap
		}
		req := pt.Req{Command: command, PID: p.PID, VAddr: p.VAddr, Mapping: after}
		stream.Queue = append(stream.Queue, tableCommand{Request: req, Group: group})
		g.Pending++
	}
	if migration == 0 {
		if s.HostPages[p.PID] == nil {
			s.HostPages[p.PID] = make(map[uint64]int)
		}
		s.HostPages[p.PID][p.VAddr]++
		if before.Valid && s.Dispatched[p.PID] {
			g.Pending += c.Middlewares.Migration.shootDownHostMapping(p, group)
		}
	}
	s.Groups[group] = g
}

func (m *pageTableMW) receive() bool {
	c := m.c
	msg, ok := c.Ports.PageTable[0].RetrieveIncoming()
	if !ok {
		return false
	}
	s := &c.State.PageTables
	i := slices.IndexFunc(s.Streams, func(stream tableStream) bool { return stream.Flight == msg.RspTo })
	if i < 0 {
		panic("driver: unmatched MMU control ACK")
	}
	stream := &s.Streams[i]
	op := stream.Queue[0]
	if err := msg.Payload.(pt.Rsp).Error; err != "" {
		panic(fmt.Sprintf("driver: %s command %d PID %d VA %#x: %s",
			msg.Src, op.Request.Command, op.Request.PID, op.Request.VAddr, err))
	}
	stream.Flight = 0
	stream.Queue = stream.Queue[1:]
	if op.Group != 0 {
		m.ack(op.Group)
	}
	return true
}

func (m *pageTableMW) ack(id uint64) {
	s := &m.c.State.PageTables
	g := s.Groups[id]
	g.Pending--
	if g.Pending != 0 {
		s.Groups[id] = g
		return
	}
	delete(s.Groups, id)
	if g.Migration != 0 {
		m.c.Middlewares.Migration.mappingsReady(g.Migration - 1)
		return
	}
	pending := s.HostPages[g.PID]
	pending[g.VAddr]--
	if pending[g.VAddr] == 0 {
		delete(pending, g.VAddr)
	}
	if len(pending) == 0 {
		delete(s.HostPages, g.PID)
	}
}
