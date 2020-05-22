package emu

import (
	"log"

	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
	"gitlab.com/akita/util/ca"
)

// A Wavefront in the emu package is a wrapper for the kernels.Wavefront
type Wavefront struct {
	*kernels.Wavefront

	pid ca.PID

	Completed  bool
	AtBarrier  bool
	inst       *insts.Inst
	scratchpad Scratchpad

	PC       uint64
	Exec     uint64
	SCC      byte
	VCC      uint64
	M0       uint32
	SRegFile []byte
	VRegFile []byte
	LDS      []byte
}

// NewWavefront returns the Wavefront that wraps the nativeWf
func NewWavefront(nativeWf *kernels.Wavefront) *Wavefront {
	wf := new(Wavefront)
	wf.Wavefront = nativeWf

	wf.SRegFile = make([]byte, 4*102)
	wf.VRegFile = make([]byte, 4*64*256)
	wf.scratchpad = make([]byte, 4096)

	return wf
}

// Inst returns the instruction that the wavefront is executing
func (wf *Wavefront) Inst() *insts.Inst {
	return wf.inst
}

// Scratchpad returns the scratchpad that is associated with the wavefront
func (wf *Wavefront) Scratchpad() Scratchpad {
	return wf.scratchpad
}

// PID returns pid
func (wf *Wavefront) PID() ca.PID {
	return wf.pid
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
//nolint:gocyclo
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
	} else if reg.RegType == insts.SCC {
		value[0] = wf.SCC
	} else if reg.RegType == insts.VCC {
		copy(value, insts.Uint64ToBytes(wf.VCC))
	} else if reg.RegType == insts.VCCLO && regCount == 1 {
		copy(value, insts.Uint32ToBytes(uint32(wf.VCC)))
	} else if reg.RegType == insts.VCCHI && regCount == 1 {
		copy(value, insts.Uint32ToBytes(uint32(wf.VCC>>32)))
	} else if reg.RegType == insts.VCCLO && regCount == 2 {
		copy(value, insts.Uint64ToBytes(wf.VCC))
	} else if reg.RegType == insts.EXEC {
		copy(value, insts.Uint64ToBytes(wf.Exec))
	} else if reg.RegType == insts.EXECLO && regCount == 2 {
		copy(value, insts.Uint64ToBytes(wf.Exec))
	} else if reg.RegType == insts.M0 {
		copy(value, insts.Uint32ToBytes(wf.M0))
	} else {
		log.Panicf("Register type %s not supported", reg.Name)
	}

	return value
}

// WriteReg returns the raw register value
//nolint:gocyclo
func (wf *Wavefront) WriteReg(
	reg *insts.Reg,
	regCount int,
	laneID int,
	data []byte,
) {
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
	} else if reg.RegType == insts.SCC {
		wf.SCC = data[0]
	} else if reg.RegType == insts.VCC {
		wf.VCC = insts.BytesToUint64(data)
	} else if reg.RegType == insts.VCCLO && regCount == 2 {
		wf.VCC = insts.BytesToUint64(data)
	} else if reg.RegType == insts.VCCLO && regCount == 1 {
		wf.VCC &= uint64(0x00000000ffffffff)
		wf.VCC |= uint64(insts.BytesToUint32(data))
	} else if reg.RegType == insts.VCCHI && regCount == 1 {
		wf.VCC &= uint64(0xffffffff00000000)
		wf.VCC |= uint64(insts.BytesToUint32(data)) << 32
	} else if reg.RegType == insts.EXEC {
		wf.Exec = insts.BytesToUint64(data)
	} else if reg.RegType == insts.EXECLO && regCount == 2 {
		wf.Exec = insts.BytesToUint64(data)
	} else if reg.RegType == insts.M0 {
		wf.M0 = insts.BytesToUint32(data)
	} else {
		log.Panicf("Register type %s not supported", reg.Name)
	}
}
