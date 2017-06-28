package kernels

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
	grid.PacketAddress = req.PacketAddress

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
				wg.CurrSizeX = int(xToAllocate)
				wg.CurrSizeY = int(yToAllocate)
				wg.CurrSizeZ = int(zToAllocate)
				wg.SizeX = int(g.Packet.WorkgroupSizeX)
				wg.SizeY = int(g.Packet.WorkgroupSizeY)
				wg.SizeZ = int(g.Packet.WorkgroupSizeZ)
				wg.IDX = wgIDX
				wg.IDY = wgIDY
				wg.IDY = wgIDZ
				xLeft -= xToAllocate
				b.spawnWorkItems(wg)
				b.formWavefronts(wg)
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

func (b *GridBuilderImpl) spawnWorkItems(wg *WorkGroup) {
	for z := 0; z < wg.CurrSizeZ; z++ {
		for y := 0; y < wg.CurrSizeY; y++ {
			for x := 0; x < wg.CurrSizeX; x++ {
				wi := new(WorkItem)
				wi.WG = wg
				wi.IDX = x
				wi.IDY = y
				wi.IDZ = z
				wg.WorkItems = append(wg.WorkItems, wi)
			}
		}
	}
}

func (b *GridBuilderImpl) formWavefronts(wg *WorkGroup) {
	var wf *Wavefront
	wavefrontSize := 64
	for i := 0; i < len(wg.WorkItems); i++ {
		if i%wavefrontSize == 0 {
			wf = NewWavefront()
			wf.FirstWiFlatID = wg.WorkItems[i].FlattenedID()
			wf.WG = wg
			wg.Wavefronts = append(wg.Wavefronts, wf)
		}
		wf.WorkItems = append(wf.WorkItems, wg.WorkItems[i])
	}
}

func min(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}
