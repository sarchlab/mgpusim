package kernels

import (
	"gitlab.com/akita/gcn3/insts"
)

// A GridBuilder is the unit that can build a grid and its internal structure
// from a kernel and its launch parameters.
type GridBuilder interface {
	//Build(hsaco *insts.HsaCo, packet *HsaKernelDispatchPacket) *Grid
	SetKernel(hsaco *insts.HsaCo, packet *HsaKernelDispatchPacket)
	NumWG() int
	NextWG() *WorkGroup
}

// NewGridBuilder creates a default grid builder
func NewGridBuilder() GridBuilder {
	return &gridBuilderImpl{}
}

type gridBuilderImpl struct {
	hsaco  *insts.HsaCo
	packet *HsaKernelDispatchPacket

	xid, yid, zid int
}

func (b *gridBuilderImpl) SetKernel(
	hsaco *insts.HsaCo,
	packet *HsaKernelDispatchPacket,
) {
	b.hsaco = hsaco
	b.packet = packet
}

func (b *gridBuilderImpl) NumWG() int {
	x := int(b.packet.GridSizeX-1)/int(b.packet.WorkgroupSizeX) + 1
	y := int(b.packet.GridSizeY-1)/int(b.packet.WorkgroupSizeY) + 1
	z := int(b.packet.GridSizeZ-1)/int(b.packet.WorkgroupSizeZ) + 1
	return x * y * z
}

func (b *gridBuilderImpl) NextWG() *WorkGroup {
	xLeft := int(b.packet.GridSizeX) - b.xid*int(b.packet.WorkgroupSizeX)
	yLeft := int(b.packet.GridSizeY) - b.yid*int(b.packet.WorkgroupSizeY)
	zLeft := int(b.packet.GridSizeZ) - b.zid*int(b.packet.WorkgroupSizeZ)

	if xLeft <= 0 || yLeft <= 0 || zLeft <= 0 {
		return nil
	}

	xToAllocate := min(xLeft, int(b.packet.WorkgroupSizeX))
	yToAllocate := min(yLeft, int(b.packet.WorkgroupSizeY))
	zToAllocate := min(zLeft, int(b.packet.WorkgroupSizeZ))

	wg := NewWorkGroup()
	wg.SetCodeObject(b.hsaco)
	wg.SizeX = int(b.packet.WorkgroupSizeX)
	wg.SizeY = int(b.packet.WorkgroupSizeY)
	wg.SizeZ = int(b.packet.WorkgroupSizeZ)
	wg.IDX = int(b.xid)
	wg.IDY = int(b.yid)
	wg.IDZ = int(b.zid)
	wg.CurrSizeX = int(xToAllocate)
	wg.CurrSizeY = int(yToAllocate)
	wg.CurrSizeZ = int(zToAllocate)

	b.spawnWorkItems(wg)
	b.formWavefronts(wg)

	b.xid++
	xLeft -= xToAllocate
	if xLeft <= 0 {
		b.xid = 0
		b.yid++
		yLeft -= yToAllocate
		if yLeft <= 0 {
			b.yid = 0
			b.zid++
		}
	}

	return wg
}

func (b *gridBuilderImpl) spawnWorkItems(wg *WorkGroup) {
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

func (b *gridBuilderImpl) formWavefronts(wg *WorkGroup) {
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
