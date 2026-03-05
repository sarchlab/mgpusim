package cu

import (
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
)

// CURegFileAccessor implements wavefront.RegFileAccessor by delegating to the
// CU's scalar and vector register files. Special registers (SCC, VCC, EXEC, M0)
// are handled via the wavefront's own fields.
type CURegFileAccessor struct {
	CU *ComputeUnit
	WF *wavefront.Wavefront
}

// ReadReg reads from the appropriate register file based on register type.
//
//nolint:gocyclo
func (a *CURegFileAccessor) ReadReg(
	reg *insts.Reg, regCount int, laneID int, waveOffset int,
) []byte {
	// Handle special registers via the wavefront fields
	switch reg.RegType {
	case insts.SCC:
		return []byte{a.WF.SCC()}
	case insts.VCC:
		return insts.Uint64ToBytes(a.WF.VCC())
	case insts.VCCLO:
		if regCount >= 2 {
			return insts.Uint64ToBytes(a.WF.VCC())
		}
		return insts.Uint32ToBytes(uint32(a.WF.VCC()))
	case insts.VCCHI:
		if regCount >= 2 {
			return insts.Uint64ToBytes(a.WF.VCC())
		}
		return insts.Uint32ToBytes(uint32(a.WF.VCC() >> 32))
	case insts.EXEC:
		return insts.Uint64ToBytes(a.WF.EXEC())
	case insts.EXECLO:
		if regCount >= 2 {
			return insts.Uint64ToBytes(a.WF.EXEC())
		}
		return insts.Uint32ToBytes(uint32(a.WF.EXEC()))
	case insts.M0:
		return insts.Uint32ToBytes(a.WF.M0)
	}

	// Handle regular SReg and VReg via register files
	var regFile RegisterFile
	if reg.IsSReg() {
		regFile = a.CU.SRegFile
	} else if reg.IsVReg() {
		regFile = a.CU.VRegFile[a.WF.SIMDID]
	} else {
		// Fallback by name for vcclo/vcchi
		if reg.Name == "vcclo" {
			if regCount >= 2 {
				return insts.Uint64ToBytes(a.WF.VCC())
			}
			return insts.Uint32ToBytes(uint32(a.WF.VCC()))
		}
		if reg.Name == "vcchi" {
			if regCount >= 2 {
				return insts.Uint64ToBytes(a.WF.VCC())
			}
			return insts.Uint32ToBytes(uint32(a.WF.VCC() >> 32))
		}
		log.Panicf("Register type %s not supported", reg.Name)
	}

	access := RegisterAccess{
		Reg:        reg,
		RegCount:   regCount,
		LaneID:     laneID,
		WaveOffset: waveOffset,
	}
	if regCount >= 2 {
		access.Data = make([]byte, reg.ByteSize*regCount)
	} else {
		access.Data = make([]byte, reg.ByteSize)
	}

	regFile.Read(access)

	return access.Data
}

// WriteReg writes to the appropriate register file based on register type.
//
//nolint:gocyclo
func (a *CURegFileAccessor) WriteReg(
	reg *insts.Reg, regCount int, laneID int, waveOffset int, data []byte,
) {
	// Handle special registers via the wavefront fields
	switch reg.RegType {
	case insts.SCC:
		a.WF.SetSCC(data[0])
		return
	case insts.VCC, insts.VCCLO:
		if regCount >= 2 || len(data) >= 8 {
			a.WF.SetVCC(insts.BytesToUint64(padTo8(data)))
		} else {
			// Write only low 32 bits
			lo := uint64(insts.BytesToUint32(data))
			hi := a.WF.VCC() & 0xFFFFFFFF00000000
			a.WF.SetVCC(hi | lo)
		}
		return
	case insts.VCCHI:
		if regCount >= 2 || len(data) >= 8 {
			a.WF.SetVCC(insts.BytesToUint64(padTo8(data)))
		} else {
			hi := uint64(insts.BytesToUint32(data)) << 32
			lo := a.WF.VCC() & 0x00000000FFFFFFFF
			a.WF.SetVCC(hi | lo)
		}
		return
	case insts.EXEC, insts.EXECLO:
		if regCount >= 2 || len(data) >= 8 {
			a.WF.SetEXEC(insts.BytesToUint64(padTo8(data)))
		} else {
			lo := uint64(insts.BytesToUint32(data))
			hi := a.WF.EXEC() & 0xFFFFFFFF00000000
			a.WF.SetEXEC(hi | lo)
		}
		return
	case insts.M0:
		a.WF.M0 = insts.BytesToUint32(data)
		return
	}

	// Handle regular SReg and VReg via register files
	var regFile RegisterFile
	if reg.IsSReg() {
		regFile = a.CU.SRegFile
	} else if reg.IsVReg() {
		regFile = a.CU.VRegFile[a.WF.SIMDID]
	} else {
		// Fallback by name for vcclo/vcchi
		if reg.Name == "vcclo" {
			if regCount >= 2 || len(data) >= 8 {
				a.WF.SetVCC(insts.BytesToUint64(padTo8(data)))
			} else {
				lo := uint64(insts.BytesToUint32(data))
				hi := a.WF.VCC() & 0xFFFFFFFF00000000
				a.WF.SetVCC(hi | lo)
			}
			return
		}
		if reg.Name == "vcchi" {
			if regCount >= 2 || len(data) >= 8 {
				a.WF.SetVCC(insts.BytesToUint64(padTo8(data)))
			} else {
				hi := uint64(insts.BytesToUint32(data)) << 32
				lo := a.WF.VCC() & 0x00000000FFFFFFFF
				a.WF.SetVCC(hi | lo)
			}
			return
		}
		log.Panicf("Register type %s not supported for write", reg.Name)
	}

	access := RegisterAccess{
		Reg:        reg,
		RegCount:   regCount,
		LaneID:     laneID,
		WaveOffset: waveOffset,
		Data:       data,
	}

	regFile.Write(access)
}

func padTo8(data []byte) []byte {
	if len(data) >= 8 {
		return data
	}
	padded := make([]byte, 8)
	copy(padded, data)
	return padded
}
