package emu

import (
	"gitlab.com/yaotsu/gcn3/disasm"
)

// A Grid is a running instance of a kernel
type Grid struct {
	CodeObject *disasm.HsaCo
	Packet     *HsaKernelDispatchPacket

	WorkGroups []*WorkGroup
}

// NewGrid creates and returns a new grid object
func NewGrid() *Grid {
	g := new(Grid)
	g.WorkGroups = make([]*WorkGroup, 0)
	return g
}

// A WorkGroup is part of the kernel that runs on one ComputeUnit
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
