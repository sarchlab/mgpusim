package emu

import (
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

	var value = make([]byte, numBytes)
	if reg.IsSReg() {
		offset := reg.RegIndex() * 4
		copy(value, wf.SRegFile[offset:offset+numBytes])
	} else if reg.IsVReg() {
		offset := laneID*256*4 + reg.RegIndex()*4
		copy(value, wf.VRegFile[offset:offset+numBytes])
	} else if reg.RegType == insts.Scc {
		value[0] = wf.SCC
	}

	return value
}

// WriteReg returns the raw register value
func (wf *Wavefront) WriteReg(reg *insts.Reg, regCount int, laneID int, data []byte) {
	numBytes := reg.ByteSize
	if regCount >= 2 {
		numBytes *= regCount
	}

	if reg.IsSReg() {
		offset := reg.RegIndex() * 4
		copy(wf.SRegFile[offset:offset+numBytes], data)
	} else if reg.IsVReg() {
		offset := laneID*256*4 + reg.RegIndex()*4
		copy(wf.VRegFile[offset:offset+numBytes], data)
	} else if reg.RegType == insts.Scc {
		wf.SCC = data[0]
	}
}
