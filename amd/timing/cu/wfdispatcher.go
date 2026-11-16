package cu

import (
	"log"

	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

// A WfDispatcher initialize wavefronts
type WfDispatcher interface {
	DispatchWf(
		wf *Wavefront,
		location protocol.WfDispatchLocation,
	)
}

// A WfDispatcherImpl will register the wavefront in wavefront pool and
// initialize all the registers
type WfDispatcherImpl struct {
	cu *ComputeUnit

	Latency           int
	scoreboardEnabled bool
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
	wf *Wavefront,
	location protocol.WfDispatchLocation,
) {
	co := wf.codeObject()
	d.setWfInfo(wf, location, co)
	d.initRegisters(wf, co)

	if d.scoreboardEnabled {
		*wf.scoreboard() = Scoreboard{}
	}
}

func (d *WfDispatcherImpl) setWfInfo(
	wf *Wavefront,
	location protocol.WfDispatchLocation,
	co *insts.CodeObjectMetadata,
) {
	wf.execution().SIMDID = location.SIMDID
	wf.execution().SRegOffset = location.SGPROffset
	wf.execution().VRegOffset = location.VGPROffset
	wf.execution().LDSOffset = location.LDSOffset
	wf.SetPC(wf.packet().KernelObject + co.Meta.KernelCodeEntryByteOffset)
	wf.SetEXEC(wf.raw().InitExecMask)
}

//nolint:gocyclo,funlen
func (d *WfDispatcherImpl) initRegisters(wf *Wavefront, value *insts.CodeObjectMetadata) {
	co := &value.Meta
	pkt := wf.packet()

	SGPRPtr := 0
	if co.EnableSgprPrivateSegmentBuffer {
		// log.Printf("EnableSgprPrivateSegmentBuffer is not supported")
		// fmt.Printf("s%d SGPRPrivateSegmentBuffer\n", SGPRPtr/4)
		SGPRPtr += 16
	}

	if co.EnableSgprDispatchPtr {
		d.cu.SRegFile.Write(RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 2, 0, wf.execution().SRegOffset,
			insts.Uint64ToBytes(wf.raw().PacketAddress),
			false,
		})

		// fmt.Printf("s%d SGPRDispatchPtr\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprQueuePtr {
		log.Printf("EnableSgprQueuePtr is not supported")
		// fmt.Printf("s%d SGPRQueuePtr\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprKernargSegmentPtr {
		d.cu.SRegFile.Write(RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 2, 0, wf.execution().SRegOffset,
			insts.Uint64ToBytes(pkt.KernargAddress),
			false,
		})

		// fmt.Printf("s%d SGPRKernelArgSegmentPtr\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprDispatchID {
		log.Printf("EnableSgprDispatchID is not supported")
		// fmt.Printf("s%d SGPRDispatchID\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprFlatScratchInit {
		log.Printf("EnableSgprFlatScratchInit is not supported")
		// fmt.Printf("s%d SGPRFlatScratchInit\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprPrivateSegmentSize {
		log.Printf("EnableSgprPrivateSegmentSize is not supported")
		// fmt.Printf("s%d SGPRPrivateSegmentSize\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkgroupCountX {
		// fmt.Printf("s%d WorkGroupCountX\n", SGPRPtr/4)

		wgCountX := (pkt.GridSizeX + uint32(pkt.WorkgroupSizeX) - 1) /
			uint32(pkt.WorkgroupSizeX)

		d.cu.SRegFile.Write(RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.execution().SRegOffset,
			insts.Uint32ToBytes(wgCountX),
			false,
		})

		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkgroupCountY {
		// fmt.Printf("s%d WorkGroupCountY\n", SGPRPtr/4)

		wgCountY := (pkt.GridSizeY + uint32(pkt.WorkgroupSizeY) - 1) /
			uint32(pkt.WorkgroupSizeY)

		d.cu.SRegFile.Write(RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.execution().SRegOffset,
			insts.Uint32ToBytes(wgCountY),
			false,
		})

		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkgroupCountZ {
		// fmt.Printf("s%d WorkGroupCountZ\n", SGPRPtr/4)

		wgCountZ := (pkt.GridSizeZ + uint32(pkt.WorkgroupSizeZ) - 1) /
			uint32(pkt.WorkgroupSizeZ)

		d.cu.SRegFile.Write(RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.execution().SRegOffset,
			insts.Uint32ToBytes(wgCountZ),
			false,
		})

		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIDX() {
		d.cu.SRegFile.Write(RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.execution().SRegOffset,
			insts.Uint32ToBytes(uint32(wf.group().raw().IDX)),
			false,
		})

		// fmt.Printf("s%d WorkGroupIdX\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIDY() {
		d.cu.SRegFile.Write(RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.execution().SRegOffset,
			insts.Uint32ToBytes(uint32(wf.group().raw().IDY)),
			false,
		})

		// fmt.Printf("s%d WorkGroupIdY\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	// Wire up work-group ID Z for genuinely 3D launches even when the (unreliable,
	// often zero for extern "C" kernels) rsrc2 enable bit is clear: a grid with
	// more than one z work-group means the kernel allocated this SGPR for
	// blockIdx.z. Without this, 3D kernels read blockIdx.z as garbage.
	if co.EnableSgprWorkGroupIDZ() ||
		(wf.group().packet() != nil &&
			wf.group().packet().GridSizeZ > uint32(wf.group().packet().WorkgroupSizeZ)) {
		d.cu.SRegFile.Write(RegisterAccess{
			0, insts.SReg(SGPRPtr / 4), 1, 0, wf.execution().SRegOffset,
			insts.Uint32ToBytes(uint32(wf.group().raw().IDZ)),
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
	for i := wf.raw().FirstWiFlatID; i < wf.raw().FirstWiFlatID+64; i++ {
		z = i / (wf.group().raw().SizeX * wf.group().raw().SizeY)
		y = i % (wf.group().raw().SizeX * wf.group().raw().SizeY) / wf.group().raw().SizeX
		x = i % (wf.group().raw().SizeX * wf.group().raw().SizeY) % wf.group().raw().SizeX
		laneID := i - wf.raw().FirstWiFlatID

		if value.Version == insts.CodeObjectV5 && co.MachineVersionMajor != 8 {
			// For gfx9+/CDNA V5 code objects (e.g. gfx942), the work-item IDs
			// are packed into v0 as v0 = (z << 20) | (y << 10) | x. This mirrors
			// the functional emulator (amd/emu/computeunit.go). Without this,
			// gfx942 kernels read threadIdx.y/z as 0, breaking every kernel
			// that uses a 2D/3D workgroup. gfx8 (GCN3) uses separate VGPRs even
			// as a V5 code object, so it takes the branch below.
			packed := uint32(x) | (uint32(y) << 10) | (uint32(z) << 20)
			d.cu.VRegFile[wf.execution().SIMDID].Write(RegisterAccess{
				0, insts.VReg(0), 1, laneID, wf.execution().VRegOffset,
				insts.Uint32ToBytes(packed),
				false,
			})
		} else {
			// For V2/V3 (GCN3) and gfx8 V5 code objects, the work-item IDs use separate
			// registers v0=x, v1=y, v2=z.
			d.cu.VRegFile[wf.execution().SIMDID].Write(RegisterAccess{
				0, insts.VReg(0), 1, laneID, wf.execution().VRegOffset,
				insts.Uint32ToBytes(uint32(x)),
				false,
			})

			if co.EnableVgprWorkItemID() > 0 {
				d.cu.VRegFile[wf.execution().SIMDID].Write(RegisterAccess{
					0, insts.VReg(1), 1, laneID, wf.execution().VRegOffset,
					insts.Uint32ToBytes(uint32(y)),
					false,
				})
			}

			if co.EnableVgprWorkItemID() > 1 {
				d.cu.VRegFile[wf.execution().SIMDID].Write(RegisterAccess{
					0, insts.VReg(2), 1, laneID, wf.execution().VRegOffset,
					insts.Uint32ToBytes(uint32(z)),
					false,
				})
			}
		}
	}
}
