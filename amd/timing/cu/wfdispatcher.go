package cu

import (
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/protocol"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
)

// A WfDispatcher initialize wavefronts
type WfDispatcher interface {
	DispatchWf(
		wf *wavefront.Wavefront,
		location protocol.WfDispatchLocation,
	)
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
	wf *wavefront.Wavefront,
	location protocol.WfDispatchLocation,
) {
	d.setWfInfo(wf, location)
	d.initRegisters(wf)
}

func (d *WfDispatcherImpl) setWfInfo(
	wf *wavefront.Wavefront,
	location protocol.WfDispatchLocation,
) {
	wf.SIMDID = location.SIMDID
	wf.SRegOffset = location.SGPROffset
	wf.VRegOffset = location.VGPROffset
	wf.LDSOffset = location.LDSOffset
	wf.PC = wf.Packet.KernelObject + wf.CodeObject.KernelCodeEntryByteOffset
	wf.EXEC = wf.InitExecMask
}

//nolint:gocyclo,funlen
func (d *WfDispatcherImpl) initRegisters(wf *wavefront.Wavefront) {
	co := wf.CodeObject
	pkt := wf.Packet

	SGPRPtr := 0
	if co.EnableSgprPrivateSegmentBuffer() {
		// log.Printf("EnableSgprPrivateSegmentBuffer is not supported")
		// fmt.Printf("s%d SGPRPrivateSegmentBuffer\n", SGPRPtr/4)
		SGPRPtr += 16
	}

	if co.EnableSgprDispatchPtr() {
		d.cu.SRegFile.Write(RegisterAccess{
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
		d.cu.SRegFile.Write(RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 2, 0, wf.SRegOffset,
			insts.Uint64ToBytes(pkt.KernargAddress),
			false,
		})

		// fmt.Printf("s%d SGPRKernelArgSegmentPtr\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprDispatchID() {
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

		d.cu.SRegFile.Write(RegisterAccess{
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

		d.cu.SRegFile.Write(RegisterAccess{
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

		d.cu.SRegFile.Write(RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.SRegOffset,
			insts.Uint32ToBytes(wgCountZ),
			false,
		})

		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIDX() {
		d.cu.SRegFile.Write(RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.SRegOffset,
			insts.Uint32ToBytes(uint32(wf.WG.IDX)),
			false,
		})

		// fmt.Printf("s%d WorkGroupIdX\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIDY() {
		d.cu.SRegFile.Write(RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.SRegOffset,
			insts.Uint32ToBytes(uint32(wf.WG.IDY)),
			false,
		})

		// fmt.Printf("s%d WorkGroupIdY\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIDZ() {
		d.cu.SRegFile.Write(RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.SRegOffset,
			insts.Uint32ToBytes(uint32(wf.WG.IDZ)),
			false,
		})

		// fmt.Printf("s%d WorkGroupIdZ\n", SGPRPtr/4)
		// SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupInfo() {
		log.Printf("EnableSgprPrivateSegmentSize is not supported")
		// SGPRPtr += 4
	}

	if co.EnableSgprPrivateSegmentWaveByteOffset() {
		log.Printf("EnableSgprPrivateSegentWaveByteOffset is not supported")
		// SGPRPtr += 4
	}

	var x, y, z int
	for i := wf.FirstWiFlatID; i < wf.FirstWiFlatID+64; i++ {
		z = i / (wf.WG.SizeX * wf.WG.SizeY)
		y = i % (wf.WG.SizeX * wf.WG.SizeY) / wf.WG.SizeX
		x = i % (wf.WG.SizeX * wf.WG.SizeY) % wf.WG.SizeX
		laneID := i - wf.FirstWiFlatID

		d.cu.VRegFile[wf.SIMDID].Write(RegisterAccess{
			0, insts.VReg(0), 1, laneID, wf.VRegOffset,
			insts.Uint32ToBytes(uint32(x)),
			false,
		})

		if co.EnableVgprWorkItemID() > 0 {
			d.cu.VRegFile[wf.SIMDID].Write(RegisterAccess{
				0, insts.VReg(1), 1, laneID, wf.VRegOffset,
				insts.Uint32ToBytes(uint32(y)),
				false,
			})
		}

		if co.EnableVgprWorkItemID() > 1 {
			d.cu.VRegFile[wf.SIMDID].Write(RegisterAccess{
				0, insts.VReg(2), 1, laneID, wf.VRegOffset,
				insts.Uint32ToBytes(uint32(z)),
				false,
			})
		}
	}
}
