package timing

import (
	"log"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/insts"
)

// A WfDispatcher initialize wavefronts
type WfDispatcher interface {
	DispatchWf(now akita.VTimeInSec, wf *Wavefront)
}

// A WfDispatcherImpl will register the wavefront in wavefront pool and
// initialize all the registers
type WfDispatcherImpl struct {
	cu *ComputeUnit

	Latency int
}

// NewWfDispatcher creates a default WfDispatcher
func NewWfDispatcher(cu *ComputeUnit) *WfDispatcherImpl {
	d := new(WfDispatcherImpl)
	d.cu = cu
	d.Latency = 0
	return d
}

// DispatchWf starts or continues a wavefront dispatching process.
func (d *WfDispatcherImpl) DispatchWf(
	now akita.VTimeInSec,
	wf *Wavefront,
) {
	d.setWfInfo(wf)
	d.initRegisters(wf)
}

func (d *WfDispatcherImpl) setWfInfo(wf *Wavefront) {
	wfInfo, ok := d.cu.WfToDispatch[wf.Wavefront]
	if !ok {
		log.Panic("Wf dispatching information is not found. This indicates " +
			"that the wavefront dispatched may not be mapped to the compute " +
			"unit before.")
	}

	wf.SIMDID = wfInfo.SIMDID
	wf.SRegOffset = wfInfo.SGPROffset
	wf.VRegOffset = wfInfo.VGPROffset
	wf.LDSOffset = wfInfo.LDSOffset
	wf.PC = wf.Packet.KernelObject + wf.CodeObject.KernelCodeEntryByteOffset
	wf.EXEC = 0xffffffffffffffff
}

func (d *WfDispatcherImpl) initRegisters(wf *Wavefront) {
	co := wf.CodeObject
	pkt := wf.Packet

	SGPRPtr := 0
	if co.EnableSgprPrivateSegmentBuffer() {
		// log.Printf("EnableSgprPrivateSegmentBuffer is not supported")
		// fmt.Printf("s%d SGPRPrivateSegmentBuffer\n", SGPRPtr/4)
		SGPRPtr += 16
	}

	if co.EnableSgprDispatchPtr() {

		d.cu.SRegFile.Write(&RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 2, 0, wf.SRegOffset,
			insts.Uint64ToBytes(wf.PacketAddress),
			false,
		})

		// fmt.Printf("s%d SGPRDispatchPtr\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprQueuePtr() {
		log.Printf("EnableSgprQueuePtr is not supported")
		// fmt.Printf("s%d SGPRQueuePtr\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprKernelArgSegmentPtr() {
		d.cu.SRegFile.Write(&RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 2, 0, wf.SRegOffset,
			insts.Uint64ToBytes(pkt.KernargAddress),
			false,
		})

		// fmt.Printf("s%d SGPRKernelArgSegmentPtr\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprDispatchId() {
		log.Printf("EnableSgprDispatchID is not supported")
		// fmt.Printf("s%d SGPRDispatchID\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprFlatScratchInit() {
		log.Printf("EnableSgprFlatScratchInit is not supported")
		// fmt.Printf("s%d SGPRFlatScratchInit\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprPrivateSegementSize() {
		log.Printf("EnableSgprPrivateSegmentSize is not supported")
		// fmt.Printf("s%d SGPRPrivateSegmentSize\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkGroupCountX() {
		// fmt.Printf("s%d WorkGroupCountX\n", SGPRPtr/4)

		wgCountX := (pkt.GridSizeX + uint32(pkt.WorkgroupSizeX) - 1) /
			uint32(pkt.WorkgroupSizeX)

		d.cu.SRegFile.Write(&RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.SRegOffset,
			insts.Uint32ToBytes(wgCountX),
			false,
		})

		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkGroupCountY() {
		// fmt.Printf("s%d WorkGroupCountY\n", SGPRPtr/4)

		wgCountY := (pkt.GridSizeY + uint32(pkt.WorkgroupSizeY) - 1) /
			uint32(pkt.WorkgroupSizeY)

		d.cu.SRegFile.Write(&RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.SRegOffset,
			insts.Uint32ToBytes(wgCountY),
			false,
		})

		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkGroupCountZ() {
		// fmt.Printf("s%d WorkGroupCountZ\n", SGPRPtr/4)

		wgCountZ := (pkt.GridSizeZ + uint32(pkt.WorkgroupSizeZ) - 1) /
			uint32(pkt.WorkgroupSizeZ)

		d.cu.SRegFile.Write(&RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.SRegOffset,
			insts.Uint32ToBytes(wgCountZ),
			false,
		})

		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIdX() {

		d.cu.SRegFile.Write(&RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.SRegOffset,
			insts.Uint32ToBytes(uint32(wf.WG.IDX)),
			false,
		})

		// fmt.Printf("s%d WorkGroupIdX\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIdY() {

		d.cu.SRegFile.Write(&RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.SRegOffset,
			insts.Uint32ToBytes(uint32(wf.WG.IDY)),
			false,
		})

		// fmt.Printf("s%d WorkGroupIdY\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIdZ() {
		d.cu.SRegFile.Write(&RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.SRegOffset,
			insts.Uint32ToBytes(uint32(wf.WG.IDZ)),
			false,
		})

		// fmt.Printf("s%d WorkGroupIdZ\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupInfo() {
		log.Printf("EnableSgprPrivateSegmentSize is not supported")
		SGPRPtr += 4
	}

	if co.EnableSgprPrivateSegmentWaveByteOffset() {
		log.Printf("EnableSgprPrivateSegentWaveByteOffset is not supported")
		SGPRPtr += 4
	}

	var x, y, z int
	for i := wf.FirstWiFlatID; i < wf.FirstWiFlatID+64; i++ {
		z = i / (wf.WG.SizeX * wf.WG.SizeY)
		y = i % (wf.WG.SizeX * wf.WG.SizeY) / wf.WG.SizeX
		x = i % (wf.WG.SizeX * wf.WG.SizeY) % wf.WG.SizeX
		laneID := i - wf.FirstWiFlatID

		d.cu.VRegFile[wf.SIMDID].Write(&RegisterAccess{
			0, insts.VReg(0), 1, laneID, wf.VRegOffset,
			insts.Uint32ToBytes(uint32(x)),
			false,
		})

		if co.EnableVgprWorkItemId() > 0 {
			d.cu.VRegFile[wf.SIMDID].Write(&RegisterAccess{
				0, insts.VReg(1), 1, laneID, wf.VRegOffset,
				insts.Uint32ToBytes(uint32(y)),
				false,
			})
		}

		if co.EnableVgprWorkItemId() > 1 {
			d.cu.VRegFile[wf.SIMDID].Write(&RegisterAccess{
				0, insts.VReg(2), 1, laneID, wf.VRegOffset,
				insts.Uint32ToBytes(uint32(z)),
				false,
			})
		}
	}

}
