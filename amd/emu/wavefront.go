package emu

import (
	"log"
	"math"

	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/kernels"
)

// A Wavefront in the emu package is a wrapper for the kernels.Wavefront
type Wavefront struct {
	*kernels.Wavefront

	pid vm.PID

	Completed  bool
	AtBarrier  bool
	inst       *insts.Inst
	scratchpad Scratchpad

	pc       uint64
	exec     uint64
	scc      byte
	vcc      uint64
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
func (wf *Wavefront) PID() vm.PID {
	return wf.pid
}

// PC returns the program counter
func (wf *Wavefront) PC() uint64 {
	return wf.pc
}

// SetPC sets the program counter
func (wf *Wavefront) SetPC(v uint64) {
	wf.pc = v
}

// EXEC returns the exec mask
func (wf *Wavefront) EXEC() uint64 {
	return wf.exec
}

// SetEXEC sets the exec mask
func (wf *Wavefront) SetEXEC(v uint64) {
	wf.exec = v
}

// SCC returns the scalar condition code
func (wf *Wavefront) SCC() byte {
	return wf.scc
}

// SetSCC sets the scalar condition code
func (wf *Wavefront) SetSCC(v byte) {
	wf.scc = v
}

// VCC returns the vector condition code
func (wf *Wavefront) VCC() uint64 {
	return wf.vcc
}

// SetVCC sets the vector condition code
func (wf *Wavefront) SetVCC(v uint64) {
	wf.vcc = v
}

// ReadOperand reads the value of an operand
func (wf *Wavefront) ReadOperand(operand *insts.Operand, laneID int) uint64 {
	switch operand.OperandType {
	case insts.RegOperand:
		buf := wf.ReadReg(operand.Register, operand.RegCount, laneID)
		if len(buf) < 8 {
			padded := make([]byte, 8)
			copy(padded, buf)
			buf = padded
		}
		return insts.BytesToUint64(buf)
	case insts.IntOperand:
		return uint64(operand.IntValue)
	case insts.FloatOperand:
		return uint64(math.Float32bits(float32(operand.FloatValue)))
	case insts.LiteralConstant:
		return uint64(operand.LiteralConstant)
	default:
		log.Panicf("Unsupported operand type: %s", operand.String())
		return 0
	}
}

// WriteOperand writes a value to an operand
func (wf *Wavefront) WriteOperand(operand *insts.Operand, laneID int, value uint64) {
	if operand.OperandType != insts.RegOperand {
		log.Panicf("Cannot write to non-register operand: %s", operand.String())
	}

	numBytes := operand.Register.ByteSize
	if operand.RegCount >= 2 {
		numBytes *= operand.RegCount
	}

	data := insts.Uint64ToBytes(value)
	wf.WriteReg(operand.Register, operand.RegCount, laneID, data[:numBytes])
}

// ReadOperandBytes reads the raw bytes of an operand
func (wf *Wavefront) ReadOperandBytes(operand *insts.Operand, laneID int, byteCount int) []byte {
	switch operand.OperandType {
	case insts.RegOperand:
		buf := wf.ReadReg(operand.Register, operand.RegCount, laneID)
		if len(buf) > byteCount {
			return buf[:byteCount]
		}
		return buf
	case insts.IntOperand:
		data := insts.Uint64ToBytes(uint64(operand.IntValue))
		return data[:byteCount]
	case insts.FloatOperand:
		data := insts.Uint64ToBytes(uint64(math.Float32bits(float32(operand.FloatValue))))
		return data[:byteCount]
	case insts.LiteralConstant:
		data := insts.Uint64ToBytes(uint64(operand.LiteralConstant))
		return data[:byteCount]
	default:
		log.Panicf("Unsupported operand type: %s", operand.String())
		return nil
	}
}

// WriteOperandBytes writes raw bytes to an operand
func (wf *Wavefront) WriteOperandBytes(operand *insts.Operand, laneID int, data []byte) {
	if operand.OperandType != insts.RegOperand {
		log.Panicf("Cannot write to non-register operand: %s", operand.String())
	}

	wf.WriteReg(operand.Register, operand.RegCount, laneID, data)
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
//
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
		value[0] = wf.scc
	} else if reg.RegType == insts.VCC {
		copy(value, insts.Uint64ToBytes(wf.vcc))
	} else if reg.RegType == insts.VCCLO && regCount == 1 {
		copy(value, insts.Uint32ToBytes(uint32(wf.vcc)))
	} else if reg.RegType == insts.VCCHI && regCount == 1 {
		copy(value, insts.Uint32ToBytes(uint32(wf.vcc>>32)))
	} else if reg.RegType == insts.VCCLO && regCount == 2 {
		copy(value, insts.Uint64ToBytes(wf.vcc))
	} else if reg.RegType == insts.EXEC {
		copy(value, insts.Uint64ToBytes(wf.exec))
	} else if reg.RegType == insts.EXECLO && regCount == 2 {
		copy(value, insts.Uint64ToBytes(wf.exec))
	} else if reg.RegType == insts.M0 {
		copy(value, insts.Uint32ToBytes(wf.M0))
	} else if reg.Name == "vcclo" {
		// Fallback for vcclo when RegType is not properly set
		if regCount == 1 {
			copy(value, insts.Uint32ToBytes(uint32(wf.vcc)))
		} else {
			copy(value, insts.Uint64ToBytes(wf.vcc))
		}
	} else if reg.Name == "vcchi" {
		// Fallback for vcchi when RegType is not properly set
		if regCount == 1 {
			copy(value, insts.Uint32ToBytes(uint32(wf.vcc>>32)))
		} else {
			copy(value, insts.Uint64ToBytes(wf.vcc))
		}
	} else {
		log.Panicf("Register type %s not supported", reg.Name)
	}

	return value
}

// WriteReg returns the raw register value
//
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
		wf.scc = data[0]
	} else if reg.RegType == insts.VCC {
		wf.vcc = insts.BytesToUint64(data)
	} else if reg.RegType == insts.VCCLO && regCount == 2 {
		wf.vcc = insts.BytesToUint64(data)
	} else if reg.RegType == insts.VCCLO && regCount == 1 {
		wf.vcc &= uint64(0x00000000ffffffff)
		wf.vcc |= uint64(insts.BytesToUint32(data))
	} else if reg.RegType == insts.VCCHI && regCount == 1 {
		wf.vcc &= uint64(0xffffffff00000000)
		wf.vcc |= uint64(insts.BytesToUint32(data)) << 32
	} else if reg.RegType == insts.EXEC {
		wf.exec = insts.BytesToUint64(data)
	} else if reg.RegType == insts.EXECLO && regCount == 2 {
		wf.exec = insts.BytesToUint64(data)
	} else if reg.RegType == insts.M0 {
		wf.M0 = insts.BytesToUint32(data)
	} else if reg.Name == "vcclo" {
		// Fallback for vcclo when RegType is not properly set
		if regCount == 1 {
			wf.vcc &= uint64(0xffffffff00000000)
			wf.vcc |= uint64(insts.BytesToUint32(data))
		} else {
			wf.vcc = insts.BytesToUint64(data)
		}
	} else if reg.Name == "vcchi" {
		// Fallback for vcchi when RegType is not properly set
		if regCount == 1 {
			wf.vcc &= uint64(0x00000000ffffffff)
			wf.vcc |= uint64(insts.BytesToUint32(data)) << 32
		} else {
			wf.vcc = insts.BytesToUint64(data)
		}
	} else {
		log.Panicf("Register type %s not supported", reg.Name)
	}
}
