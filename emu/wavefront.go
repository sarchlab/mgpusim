package emu

import (
	"log"

	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
)

// A Wavefront in the emu package is a wrapper for the kernels.Wavefront
type Wavefront struct {
	*kernels.Wavefront

	CodeObject    *insts.HsaCo
	Packet        *kernels.HsaKernelDispatchPacket
	PacketAddress uint64

	Completed  bool
	AtBarrier  bool
	inst       *insts.Inst
	scratchpad Scratchpad

	PC       uint64
	Exec     uint64
	SCC      byte
	VCC      uint64
	SRegFile []byte
	VRegFile []byte
}

// NewWavefront returns the Wavefront that wraps the nativeWf
func NewWavefront(nativeWf *kernels.Wavefront) *Wavefront {
	wf := new(Wavefront)
	wf.Wavefront = nativeWf

	if nativeWf != nil {
		wf.CodeObject = nativeWf.WG.Grid.CodeObject
		wf.Packet = nativeWf.WG.Grid.Packet
		wf.PacketAddress = nativeWf.WG.Grid.PacketAddress
	}

	wf.SRegFile = make([]byte, 4*102)
	wf.VRegFile = make([]byte, 4*64*256)
	wf.scratchpad = make([]byte, 4096)

	return wf
}

// Inst returns the instruction that the wavefront is executing
func (wf *Wavefront) Inst() *insts.Inst {
	return wf.inst
}

// Scratchpad returns the sratchpad that is associated with the wavefront
func (wf *Wavefront) Scratchpad() Scratchpad {
	return wf.scratchpad
}

// SRegValue returns s(i)'s value
func (wf *Wavefront) SRegValue(i int) uint32 {
	return insts.BytesToUint32(wf.SRegFile[i*4 : i*4+4])
}

// VRegValue returns the value of v(i) of a certain lain
func (wf *Wavefront) VRegValue(lane int, i int) uint32 {
	offset := lane*1024 + i*4
	return insts.BytesToUint32(wf.VRegFile[offset : offset+4])
}

// ReadReg returns the raw register value
func (wf *Wavefront) ReadReg(reg *insts.Reg, regCount int, laneID int) []byte {
	numBytes := reg.ByteSize
	if regCount >= 2 {
		numBytes *= regCount
	}

	// There are some concerns in terms of reading VCC and EXEC (64 or 32? And how to decide?)
	var value = make([]byte, numBytes)
	if reg.IsSReg() {
		offset := reg.RegIndex() * 4
		copy(value, wf.SRegFile[offset:offset+numBytes])
	} else if reg.IsVReg() {
		offset := laneID*256*4 + reg.RegIndex()*4
		copy(value, wf.VRegFile[offset:offset+numBytes])
	} else if reg.RegType == insts.Scc {
		value[0] = wf.SCC
	} else if reg.RegType == insts.Vcc {
		copy(value, insts.Uint64ToBytes(wf.VCC))
	} else if reg.RegType == insts.VccLo && regCount == 1 {
		copy(value, insts.Uint32ToBytes(uint32(wf.VCC)))
	} else if reg.RegType == insts.VccHi && regCount == 1 {
		copy(value, insts.Uint32ToBytes(uint32(wf.VCC>>32)))
	} else if reg.RegType == insts.VccLo && regCount == 2 {
		copy(value, insts.Uint64ToBytes(wf.VCC))
	} else if reg.RegType == insts.Exec {
		copy(value, insts.Uint64ToBytes(wf.Exec))
	} else if reg.RegType == insts.ExecLo && regCount == 2 {
		copy(value, insts.Uint64ToBytes(wf.Exec))
	} else {
		log.Panicf("Register type %s not supported", reg.Name)
	}

	return value
}

// WriteReg returns the raw register value
func (wf *Wavefront) WriteReg(reg *insts.Reg, regCount int, laneID int, data []byte) {
	numBytes := reg.ByteSize
	if regCount >= 2 {
		numBytes *= regCount
	}

	// There are some concerns in terms of reading VCC and EXEC (64 or 32? And how to decide?)
	if reg.IsSReg() {
		offset := reg.RegIndex() * 4
		copy(wf.SRegFile[offset:offset+numBytes], data)
	} else if reg.IsVReg() {
		offset := laneID*256*4 + reg.RegIndex()*4
		copy(wf.VRegFile[offset:offset+numBytes], data)
	} else if reg.RegType == insts.Scc {
		wf.SCC = data[0]
	} else if reg.RegType == insts.Vcc {
		wf.VCC = insts.BytesToUint64(data)
	} else if reg.RegType == insts.VccLo && regCount == 2 {
		wf.VCC = insts.BytesToUint64(data)
	} else if reg.RegType == insts.VccLo && regCount == 1 {
		wf.VCC &= uint64(0x00000000ffffffff)
		wf.VCC |= uint64(insts.BytesToUint32(data))
	} else if reg.RegType == insts.VccHi && regCount == 1 {
		wf.VCC &= uint64(0xffffffff00000000)
		wf.VCC |= uint64(insts.BytesToUint32(data)) << 32
	} else if reg.RegType == insts.Exec {
		wf.Exec = insts.BytesToUint64(data)
	} else if reg.RegType == insts.ExecLo && regCount == 2 {
		wf.Exec = insts.BytesToUint64(data)
	} else {
		log.Panicf("Register type %s not supported", reg.Name)
	}

}
