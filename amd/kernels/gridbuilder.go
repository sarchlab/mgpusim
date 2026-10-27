package kernels

import "github.com/sarchlab/mgpusim/v5/amd/insts"

// KernelLaunchInfo includes the necessary information to launch a kernel.
type KernelLaunchInfo struct {
	CodeObject *insts.KernelCodeObject
	Packet     *HsaKernelDispatchPacket
	PacketAddr uint64
	Selection  WGSelection
}

// A GridBuilder is the unit that can build a grid and its internal structure
// from a kernel and its launch parameters.
type GridBuilder interface {
	SetKernel(info KernelLaunchInfo)
	NumWG() int
	NextWG() *WorkGroup
	Skip(n int)
	Snapshot() GridBuilderState
	Restore(GridBuilderState)
}

// NewGridBuilder creates a default grid builder
func NewGridBuilder(ids interface{ NewID() uint64 }) GridBuilder {
	return &gridBuilderImpl{ids: ids}
}

type gridBuilderImpl struct {
	ids        interface{ NewID() uint64 }
	hsaco      *insts.KernelCodeObject
	packet     *HsaKernelDispatchPacket
	selection  WGSelection
	packetAddr uint64
	numWG      int

	xid, yid, zid int
}

func (b *gridBuilderImpl) SetKernel(
	info KernelLaunchInfo,
) {
	b.hsaco = info.CodeObject
	b.packet = info.Packet
	b.packetAddr = info.PacketAddr
	b.selection = info.Selection
	b.xid = 0
	b.yid = 0
	b.zid = 0

	b.countWG()
}

func (b *gridBuilderImpl) Skip(n int) {
	for i := 0; i < n; i++ {
		b.NextWG()
	}
}

func (b *gridBuilderImpl) countWG() {
	x := int(b.packet.GridSizeX-1)/int(b.packet.WorkgroupSizeX) + 1
	y := int(b.packet.GridSizeY-1)/int(b.packet.WorkgroupSizeY) + 1
	z := int(b.packet.GridSizeZ-1)/int(b.packet.WorkgroupSizeZ) + 1

	b.numWG = int(b.selection.Count(uint64(x) * uint64(y) * uint64(z)))
}

func (b *gridBuilderImpl) NumWG() int {
	return b.numWG
}

func (b *gridBuilderImpl) NextWG() *WorkGroup {
	x := (uint64(b.packet.GridSizeX)-1)/uint64(b.packet.WorkgroupSizeX) + 1
	y := (uint64(b.packet.GridSizeY)-1)/uint64(b.packet.WorkgroupSizeY) + 1
	wg := NewWorkGroup(b.ids.NewID())

	for {
		xLeft := int(b.packet.GridSizeX) - b.xid*int(b.packet.WorkgroupSizeX)
		yLeft := int(b.packet.GridSizeY) - b.yid*int(b.packet.WorkgroupSizeY)
		zLeft := int(b.packet.GridSizeZ) - b.zid*int(b.packet.WorkgroupSizeZ)

		if xLeft <= 0 || yLeft <= 0 || zLeft <= 0 {
			return nil
		}

		xToAllocate := min(xLeft, int(b.packet.WorkgroupSizeX))
		yToAllocate := min(yLeft, int(b.packet.WorkgroupSizeY))
		zToAllocate := min(zLeft, int(b.packet.WorkgroupSizeZ))

		wg.Packet = b.packet
		wg.CodeObject = b.hsaco
		wg.SizeX = int(b.packet.WorkgroupSizeX)
		wg.SizeY = int(b.packet.WorkgroupSizeY)
		wg.SizeZ = int(b.packet.WorkgroupSizeZ)
		wg.IDX = b.xid
		wg.IDY = b.yid
		wg.IDZ = b.zid
		wg.CurrSizeX = xToAllocate
		wg.CurrSizeY = yToAllocate
		wg.CurrSizeZ = zToAllocate

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

		if !b.selection.Partitioned {
			break
		}

		if b.selection.Includes(uint64(wg.IDX) + uint64(wg.IDY)*x + uint64(wg.IDZ)*x*y) {
			break
		}
	}

	b.spawnWorkItems(wg)
	b.formWavefronts(wg)

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
	for i, wi := range wg.WorkItems {
		wg := wi.WG
		inWGID := wi.IDZ*wg.SizeX*wg.SizeY + wi.IDY*wg.SizeX + wi.IDX
		if inWGID%wavefrontSize == 0 {
			wf = NewWavefront(b.ids.NewID())
			wf.FirstWiFlatID = wg.WorkItems[i].FlattenedID()
			wf.CodeObject = b.hsaco
			wf.Packet = b.packet
			wf.PacketAddress = b.packetAddr
			wf.WG = wg
			wg.Wavefronts = append(wg.Wavefronts, wf)
		}
		wf.WorkItems = append(wf.WorkItems, wi)
		wf.InitExecMask |= 1 << uint32(inWGID%wavefrontSize)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GridBuilderState records the next work-group position and launch data. The
// ID allocator belongs to the rebuilt simulation and is never serialized.
type GridBuilderState struct {
	CodeObject    insts.CodeObjectMetadata
	Packet        PacketValue
	Selection     WGSelection
	PacketAddress uint64
	NumWG         int
	X, Y, Z       int
}

func (b *gridBuilderImpl) Snapshot() GridBuilderState {
	return GridBuilderState{
		CodeObject:    insts.CodeObjectMetadataSnapshot(b.hsaco),
		Packet:        PacketSnapshot(b.packet),
		Selection:     b.selection,
		PacketAddress: b.packetAddr,
		NumWG:         b.numWG,
		X:             b.xid,
		Y:             b.yid,
		Z:             b.zid,
	}
}

func (b *gridBuilderImpl) Restore(state GridBuilderState) {
	b.hsaco = state.CodeObject.Restore()
	b.packet = state.Packet.Restore()
	b.selection = state.Selection
	b.packetAddr = state.PacketAddress
	b.numWG = state.NumWG
	b.xid, b.yid, b.zid = state.X, state.Y, state.Z
}

// NextWorkGroup advances a canonical grid cursor. The pointer graph it returns
// is a temporary view; callers persist WorkGroupValue when retaining the work.
func NextWorkGroup(state *GridBuilderState, ids interface{ NewID() uint64 }) *WorkGroup {
	b := &gridBuilderImpl{ids: ids}
	b.Restore(*state)
	wg := b.NextWG()
	state.X, state.Y, state.Z = b.xid, b.yid, b.zid
	return wg
}
