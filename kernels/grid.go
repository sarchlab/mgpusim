package kernels

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/insts"
)

// A Grid is a running instance of a kernel.
type Grid struct {
	CodeObject    *insts.HsaCo
	Packet        *HsaKernelDispatchPacket
	PacketAddress uint64

	WorkGroups []*WorkGroup
	WorkItems  []*WorkItem
}

// NewGrid creates and returns a new grid object.
func NewGrid() *Grid {
	g := new(Grid)
	g.WorkGroups = make([]*WorkGroup, 0)
	g.WorkItems = make([]*WorkItem, 0)
	return g
}

// A WorkGroup is part of the kernel that runs on one ComputeUnit.
type WorkGroup struct {
	UID                             string
	CodeObject                      *insts.HsaCo
	Packet                          *HsaKernelDispatchPacket
	PacketAddress                   uint64
	SizeX, SizeY, SizeZ             int
	IDX, IDY, IDZ                   int
	CurrSizeX, CurrSizeY, CurrSizeZ int

	Wavefronts []*Wavefront
	WorkItems  []*WorkItem
}

// NewWorkGroup creates a workgroup object.
func NewWorkGroup() *WorkGroup {
	wg := new(WorkGroup)
	wg.UID = sim.GetIDGenerator().Generate()
	wg.Wavefronts = make([]*Wavefront, 0)
	wg.WorkItems = make([]*WorkItem, 0)
	return wg
}

// A Wavefront is a collection of work-items.
type Wavefront struct {
	UID           string
	CodeObject    *insts.HsaCo
	Packet        *HsaKernelDispatchPacket
	PacketAddress uint64
	FirstWiFlatID int
	WG            *WorkGroup
	InitExecMask  uint64

	WorkItems []*WorkItem
	//for sampling
	Finishtime sim.VTimeInSec
	Issuetime  sim.VTimeInSec
}

// NewWavefront returns a new Wavefront.
func NewWavefront() *Wavefront {
	wf := new(Wavefront)
	wf.UID = sim.GetIDGenerator().Generate()
	wf.WorkItems = make([]*WorkItem, 0, 64)
	return wf
}

// A WorkItem defines a set of vector registers.
type WorkItem struct {
	WG            *WorkGroup
	IDX, IDY, IDZ int
}

// FlattenedID returns the work-item flattened ID.
func (wi *WorkItem) FlattenedID() int {
	return wi.IDX + wi.IDY*wi.WG.SizeX + wi.IDZ*wi.WG.SizeX*wi.WG.SizeY
}
