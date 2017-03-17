package emu

// A GridBuilder is the unit that can build a grid and its internal structure
// from a kernel and its launch parameters.
type GridBuilder interface {
	Build(req *LaunchKernelReq) *Grid
}

// GridBuilderImpl provides a default implementation of the GridBuilder
// interface
type GridBuilderImpl struct {
}

// Build function creates a grid according a kernel launch. It also builds
// all the work-groups.
func (b *GridBuilderImpl) Build(req *LaunchKernelReq) *Grid {
	grid := NewGrid()

	grid.Packet = req.Packet
	grid.CodeObject = req.HsaCo

	b.spawnWorkGroups(grid)

	return grid
}

func (b *GridBuilderImpl) spawnWorkGroups(g *Grid) {
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

func min(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}
