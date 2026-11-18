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
	Parent  uint64
}

type tableStream struct {
	Roots  map[vm.PID]bool
	Queue  []tableCommand
	Flight uint64
	Task   uint64
}

type mappingGroup struct {
	PID       vm.PID
	VAddr     uint64
	Pending   int
	Shootdown vm.Page
}

// pageView is the mapping after the commands already queued for this VA.
// Explicit advice follows the allocation. Lazy permissions belong to its
// current residence and are discarded on movement or pin expiry.
type pageView struct {
	Page           vm.Page
	Explicit, Lazy []int
}

func addGPU(gpus []int, gpu int) []int {
	if slices.Contains(gpus, gpu) {
		return gpus
	}
	return append(slices.Clone(gpus), gpu)
}

func (v pageView) withPage(p vm.Page) pageView {
	if !p.Valid {
		return pageView{}
	}
	if p.PAddr != v.Page.PAddr || p.DeviceID != v.Page.DeviceID || (v.Page.IsPinned && !p.IsPinned) {
		v.Lazy = nil
	}
	v.Page = p
	return v
}

type pageTableState struct {
	// HostPages gates dependent dispatch. PendingPages also includes
	// fault-installed PTEs and migration updates that only Release awaits.
	HostPages    map[vm.PID]map[uint64]bool
	Views        map[vm.PID]map[uint64]pageView
	Streams      []tableStream
	Groups       map[uint64]mappingGroup
	PendingPages map[vm.PID]map[uint64]int
	Dispatched   map[vm.PID]bool
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
	s.Views = make(map[vm.PID]map[uint64]pageView)
	s.Groups = make(map[uint64]mappingGroup)
	s.HostPages = make(map[vm.PID]map[uint64]bool)
	s.PendingPages = make(map[vm.PID]map[uint64]int)
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
		msg := messaging.Msg{ID: stream.Flight, Src: port.AsRemote(), Dst: c.Spec.PageTables[i].Control,
			Payload: stream.Queue[0].Request}
		stream.Task = c.TaskTracker().StartReqOut(c, msg, stream.Queue[0].Parent)
		port.Send(msg)
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
	var order []key
	merged := make(map[key]pageView)
	for _, change := range changes {
		page := change.After
		if !page.Valid {
			page = change.Before
		}
		k := key{page.PID, page.VAddr}
		view, ok := merged[k]
		if !ok {
			order = append(order, k)
			view = m.c.State.PageTables.Views[k.pid][k.va]
		}
		if change.AccessedBy != 0 {
			view.Explicit = addGPU(view.Explicit, change.AccessedBy)
		} else {
			view = view.withPage(change.After)
		}
		merged[k] = view
	}
	for _, k := range order {
		before := m.c.State.PageTables.Views[k.pid][k.va]
		g := mappingGroup{PID: k.pid, VAddr: k.va}
		if before.Page.Valid && before.Page != merged[k].Page && m.c.State.PageTables.Dispatched[k.pid] {
			g.Shootdown = before.Page
		}
		m.publish(g, merged[k], 0)
		s := &m.c.State.PageTables
		if s.PendingPages[k.pid][k.va] != 0 {
			// Advice can promote a lazy Map that is already in flight.
			if s.HostPages[k.pid] == nil {
				s.HostPages[k.pid] = make(map[uint64]bool)
			}
			s.HostPages[k.pid][k.va] = true
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
				if pending[p] {
					return false
				}
			}
		}
	}
	return true
}

func (m *pageTableMW) contains(target int, v pageView) bool {
	p := v.Page
	return p.Valid && (target == 0 || uint64(target) == p.DeviceID ||
		m.c.Spec.GlobalGPUPageTable && p.DeviceID != 0 ||
		slices.Contains(v.Explicit, target) || slices.Contains(v.Lazy, target))
}

// publish compares the projected views, including commands still in flight.
// The streams serialize a lazy Map and a later Unmap of the very same PTE.
func (m *pageTableMW) publish(g mappingGroup, next pageView, parent uint64) {
	c := m.c
	s := &c.State.PageTables
	before := s.Views[g.PID][g.VAddr]
	if next.Page.Valid {
		if s.Views[g.PID] == nil {
			s.Views[g.PID] = make(map[uint64]pageView)
		}
		s.Views[g.PID][g.VAddr] = next
	} else {
		delete(s.Views[g.PID], g.VAddr)
		if len(s.Views[g.PID]) == 0 {
			delete(s.Views, g.PID)
		}
	}
	var group uint64
	for i := range s.Streams {
		had, has := m.contains(i, before), m.contains(i, next)
		if !had && !has || had && has && before.Page == next.Page {
			continue
		}
		if group == 0 {
			group = c.NewID()
		}
		command := pt.Map
		if had {
			command = pt.Remap
		}
		if !has {
			command = pt.Unmap
		}
		req := pt.Req{Command: command, PID: g.PID, VAddr: g.VAddr, Mapping: next.Page}
		m.enqueue(i, tableCommand{Request: req, Group: group, Parent: parent})
		g.Pending++
	}
	if group == 0 {
		return
	}
	if s.PendingPages[g.PID] == nil {
		s.PendingPages[g.PID] = make(map[uint64]int)
	}
	s.PendingPages[g.PID][g.VAddr]++
	s.Groups[group] = g
}

func (m *pageTableMW) enqueue(target int, cmd tableCommand) {
	c := m.c
	stream := &c.State.PageTables.Streams[target]
	pid := cmd.Request.PID
	if !stream.Roots[pid] {
		pool := c.Spec.PageTables[target]
		n := uint64(len(stream.Roots))
		if n >= pool.PoolNodes {
			panic("driver: MMU root pool exhausted")
		}
		// Roots descend while intermediate nodes are allocated upward.
		root := pool.PoolBase + (pool.PoolNodes-1-n)*4096
		req := pt.Req{Command: pt.RegisterRoot, PID: pid, Root: root}
		stream.Queue = append(stream.Queue, tableCommand{Request: req, Parent: cmd.Parent})
		stream.Roots[pid] = true
	}
	stream.Queue = append(stream.Queue, cmd)
}

// Remote translations in these platforms can reach every physical peer and
// the CPU through RDMA. No local control ACK is on the fault response path.
func (m *pageTableMW) fillRemote(page vm.Page, gpu int, parent uint64) {
	v := m.c.State.PageTables.Views[page.PID][page.VAddr]
	if !v.Page.Valid || m.contains(gpu, v) {
		return
	}
	v.Lazy = addGPU(v.Lazy, gpu)
	m.publish(mappingGroup{PID: page.PID, VAddr: page.VAddr}, v, parent)
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
	c.TaskTracker().EndReqOut(stream.Task)
	stream.Task = 0
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
	if g.Shootdown.Valid {
		g.Pending = m.c.Middlewares.Migration.shootDownHostMapping(g.Shootdown, id)
		g.Shootdown = vm.Page{}
		s.Groups[id] = g
		return
	}
	delete(s.Groups, id)
	pending := s.PendingPages[g.PID]
	pending[g.VAddr]--
	if pending[g.VAddr] == 0 {
		delete(pending, g.VAddr)
		delete(s.HostPages[g.PID], g.VAddr)
		if len(s.HostPages[g.PID]) == 0 {
			delete(s.HostPages, g.PID)
		}
	}
	if len(pending) == 0 {
		delete(s.PendingPages, g.PID)
	}
	migration := m.c.Middlewares.Migration
	if pi, ok := migration.findPage(g.PID, g.VAddr); ok {
		migration.mappingsReady(pi)
	}
}
