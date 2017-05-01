package emu

import (
	"encoding/binary"
	"log"

	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
)

// RegInitiator can initiate the CU's register when starting a workgroup
type RegInitiator struct {
	CU     *ComputeUnit
	WG     *kernels.WorkGroup
	CO     *insts.HsaCo
	Packet *kernels.HsaKernelDispatchPacket
}

// InitRegs initiate the CU's register initial state
func (i *RegInitiator) InitRegs() {
	i.initSRegs()
	i.initVRegs()
	i.initMiscRegs()
}

func (i *RegInitiator) initSRegs() {
	numWi := i.WG.SizeX * i.WG.SizeY * i.WG.SizeZ
	for wiID := 0; wiID < numWi; wiID += i.CU.WiPerWf {
		i.initSRegsForWf(wiID)
	}
}

func (i *RegInitiator) initSRegsForWf(wiID int) {
	count := 0
	if i.CO.EnableSgprPrivateSegmentWaveByteOffset() {
		log.Panic("Initializing register PrivateSegmentWaveByteOffset is not supported")
		count += 4
	}

	if i.CO.EnableSgprDispatchPtr() {
		reg := insts.SReg(count)
		bytes := make([]byte, 8)
		binary.PutUvarint(bytes, uint64(0))
		i.CU.WriteReg(reg, wiID, bytes)
		count += 2
	}

	if i.CO.EnableSgprQueuePtr() {
		log.Println("Initializing register QueuePtr is not supported")
		count += 2
	}

	if i.CO.EnableSgprKernelArgSegmentPtr() {
		reg := insts.SReg(count)
		bytes := insts.Uint64ToBytes(i.Packet.KernargAddress)
		i.CU.WriteReg(reg, wiID, bytes)
		count += 2
	}

	if i.CO.EnableSgprDispatchId() {
		log.Println("Initializing register DispatchId is not supported")
		count += 2
	}

	if i.CO.EnableSgprFlatScratchInit() {
		log.Println("Initializing register FlatScratchInit is not supported")
		count += 2
	}

	if i.CO.EnableSgprPrivateSegementSize() {
		log.Println("Initializing register PrivateSegementSize is not supported")
		count++
	}

	if i.CO.EnableSgprGridWorkGroupCountX() {
		log.Println("Initializing register GridWorkGroupCountX is not supported")
		count++
	}

	if i.CO.EnableSgprGridWorkGroupCountY() {
		log.Println("Initializing register GridWorkGroupCountY is not supported")
		count++
	}

	if i.CO.EnableSgprGridWorkGroupCountZ() {
		log.Println("Initializing register GridWorkGroupCountZ is not supported")
		count++
	}

	if i.CO.EnableSgprWorkGroupIdX() {
		reg := insts.SReg(count)
		bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, uint32(i.WG.IDX))
		i.CU.WriteReg(reg, wiID, bytes)
		count++
	}

	if i.CO.EnableSgprWorkGroupIdY() {
		reg := insts.SReg(count)
		bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, uint32(i.WG.IDY))
		i.CU.WriteReg(reg, wiID, bytes)
		count++
	}

	if i.CO.EnableSgprWorkGroupIdZ() {
		reg := insts.SReg(count)
		bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, uint32(i.WG.IDZ))
		i.CU.WriteReg(reg, wiID, bytes)
		count++
	}

	if i.CO.EnableSgprWorkGroupInfo() {
		log.Println("Initializing register GridWorkGroupInfo is not supported")
		count++
	}

	if i.CO.EnableSgprPrivateSegmentWaveByteOffset() {
		log.Println("Initializing register PrivateSegmentWaveByteOffset is not supported")
		count++
	}

}

func (i *RegInitiator) initVRegs() {
	for x := 0; x < i.WG.SizeX; x++ {
		for y := 0; y < i.WG.SizeY; y++ {
			for z := 0; z < i.WG.SizeZ; z++ {
				i.initVRegsForWI(
					x, y, z, x+y*i.WG.SizeX+z*i.WG.SizeX*i.WG.SizeY)
			}
		}
	}
}

func (i *RegInitiator) initVRegsForWI(
	wiIDX, wiIDY, wiIDZ, wiFlatID int) {
	reg := insts.VReg(0)
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, uint32(wiIDX))
	i.CU.WriteReg(reg, wiFlatID, bytes)

	if i.CO.EnableVgprWorkItemId() > 0 {
		reg = insts.VReg(1)
		bytes = make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, uint32(wiIDY))
		i.CU.WriteReg(reg, wiFlatID, bytes)
	}
	if i.CO.EnableVgprWorkItemId() > 1 {
		reg = insts.VReg(2)
		bytes = make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, uint32(wiIDZ))
		i.CU.WriteReg(reg, wiFlatID, bytes)
	}
}

func (i *RegInitiator) initMiscRegs() {
	numWi := i.WG.SizeX * i.WG.SizeY * i.WG.SizeZ
	for wiID := 0; wiID < numWi; wiID += i.CU.WiPerWf {
		reg := insts.Regs[insts.Pc]
		bytes := insts.Uint64ToBytes(
			i.Packet.KernelObject + i.CO.KernelCodeEntryByteOffset)
		i.CU.WriteReg(reg, wiID, bytes)

		reg = insts.Regs[insts.Exec]
		bytes = make([]byte, 8)
		binary.LittleEndian.PutUint64(bytes, uint64(0xffffffff))
		i.CU.WriteReg(reg, wiID, bytes)
	}
}
