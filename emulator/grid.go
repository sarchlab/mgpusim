package emulator

import (
	"gitlab.com/yaotsu/gcn3/disasm"
)

// A Grid is a running instance of a kernel
type Grid struct {
	CodeObject *disasm.HsaCo
	Packet     *HsaKernelDispatchPacket

	WorkGroups        []*WorkGroup
	WorkGroupsRunning []*WorkGroup
	WorkGroupsDone    []*WorkGroup
}

// NewGrid creates and returns a new grid object
func NewGrid() *Grid {
	g := new(Grid)
	g.WorkGroups = make([]*WorkGroup, 0)
	g.WorkGroupsRunning = make([]*WorkGroup, 0)
	g.WorkGroupsDone = make([]*WorkGroup, 0)
	return g
}

func min(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

// SpawnWorkGroups will create all the workgroups that need to be executed
// in a grid. This function only creates the data stucture of workgroup.
// The details of the workgroups will be created only when they are mapped to
// ComputeUnits.
func (g *Grid) SpawnWorkGroups() {
	xLeft := g.Packet.GridSizeX
	yLeft := g.Packet.GridSizeY
	zLeft := g.Packet.GridSizeZ

	wgIDX := 0
	wgIDY := 0
	wgIDZ := 0
	for zLeft > 0 {
		zToAllocate := min(zLeft, uint32(g.Packet.WorkgroupSizeZ))
		for yLeft > 0 {
			yToAllocate := min(yLeft, uint32(g.Packet.WorkgroupSizeY))
			for xLeft > 0 {
				xToAllocate := min(xLeft, uint32(g.Packet.WorkgroupSizeX))
				wg := NewWorkGroup()
				wg.Grid = g
				wg.SizeX = int(xToAllocate)
				wg.SizeY = int(yToAllocate)
				wg.SizeZ = int(zToAllocate)
				wg.IDX = wgIDX
				wg.IDY = wgIDY
				wg.IDY = wgIDZ
				xLeft -= xToAllocate
				g.WorkGroups = append(g.WorkGroups, wg)
				wgIDX++
			}
			yLeft -= yToAllocate
			xLeft = g.Packet.GridSizeX
			wgIDY++
		}
		zLeft -= zToAllocate
		yLeft = g.Packet.GridSizeY
		wgIDZ++
	}
}

// A Workgroup is part of the kernel that runs on one ComputeUnit
type WorkGroup struct {
	Grid                *Grid
	SizeX, SizeY, SizeZ int
	IDX, IDY, IDZ       int
}

// NewWorkGroup creates a workgroup object
func NewWorkGroup() *WorkGroup {
	wg := new(WorkGroup)
	return wg
}
