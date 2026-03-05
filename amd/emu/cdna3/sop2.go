package cdna3

import (
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
)

//nolint:gocyclo,funlen
func (u *ALU) runSOP2(state emu.InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSADDU32(state)
	case 1:
		u.runSSUBU32(state)
	case 2:
		u.runSADDI32(state)
	case 3:
		u.runSSUBI32(state)
	case 4:
		u.runSADDCU32(state)
	case 5:
		u.runSSUBBU32(state)
	case 6:
		u.runSMINI32(state)
	case 7:
		u.runSMINU32(state)
	case 8:
		u.runSMAXI32(state)
	case 9:
		u.runSMAXU32(state)
	case 10:
		u.runSCSELECTB32(state)
	case 11:
		u.runSCSELECTB64(state)
	case 12:
		u.runSANDB32(state)
	case 13:
		u.runSANDB64(state)
	case 14:
		u.runSORB32(state)
	case 15:
		u.runSORB64(state)
	case 16:
		u.runSXORB32(state)
	case 17:
		u.runSXORB64(state)
	case 18:
		u.runSANDN2B32(state)
	case 19:
		u.runSANDN2B64(state)
	case 20:
		u.runSORN2B32(state)
	case 21:
		u.runSORN2B64(state)
	case 28:
		u.runSLSHLB32(state)
	case 29:
		u.runSLSHLB64(state)
	case 30:
		u.runSLSHRB32(state)
	case 31:
		u.runSLSHRB64(state)
	case 32:
		u.runSASHRI32(state)
	case 33:
		u.runSASHRI64(state)
	case 34:
		u.runSBFMB32(state)
	case 36:
		u.runSMULI32(state)
	case 37:
		u.runSBFEU32(state)
	case 38:
		u.runSBFEI32(state)
	case 44:
		u.runSMULHIU32(state)
	default:
		log.Panicf("Opcode %d for SOP2 format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runSADDU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0) & 0xFFFFFFFF
	src1 := state.ReadOperand(inst.Src1, 0) & 0xFFFFFFFF
	sum := src0 + src1
	state.WriteOperand(inst.Dst, 0, sum&0xFFFFFFFF)
	if sum > 0xFFFFFFFF {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSSUBU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0) & 0xFFFFFFFF
	src1 := state.ReadOperand(inst.Src1, 0) & 0xFFFFFFFF
	state.WriteOperand(inst.Dst, 0, (src0-src1)&0xFFFFFFFF)
	if src1 > src0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSADDI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, 0)))
	sum := int64(src0) + int64(src1)
	state.WriteOperand(inst.Dst, 0, uint64(emu.Int32ToBits(int32(sum))))
	if sum > 0x7FFFFFFF || sum < -0x80000000 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSSUBI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, 0)))
	diff := int64(src0) - int64(src1)
	state.WriteOperand(inst.Dst, 0, uint64(emu.Int32ToBits(int32(diff))))
	if diff > 0x7FFFFFFF || diff < -0x80000000 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSADDCU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0) & 0xFFFFFFFF
	src1 := state.ReadOperand(inst.Src1, 0) & 0xFFFFFFFF
	scc := state.SCC()
	sum := src0 + src1 + uint64(scc)
	state.WriteOperand(inst.Dst, 0, sum&0xFFFFFFFF)
	if sum > 0xFFFFFFFF {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSSUBBU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0) & 0xFFFFFFFF
	src1 := state.ReadOperand(inst.Src1, 0) & 0xFFFFFFFF
	scc := state.SCC()
	state.WriteOperand(inst.Dst, 0, (src0-src1-uint64(scc))&0xFFFFFFFF)
	if src1+uint64(scc) > src0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSMINI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	src0i := emu.AsInt32(uint32(src0))
	src1i := emu.AsInt32(uint32(src1))
	if src0i < src1i {
		state.WriteOperand(inst.Dst, 0, uint64(emu.Int32ToBits(src0i)))
		state.SetSCC(1)
	} else {
		state.WriteOperand(inst.Dst, 0, uint64(emu.Int32ToBits(src1i)))
		state.SetSCC(0)
	}
}

func (u *ALU) runSMINU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	if src0 < src1 {
		state.WriteOperand(inst.Dst, 0, src0)
		state.SetSCC(1)
	} else {
		state.WriteOperand(inst.Dst, 0, src1)
		state.SetSCC(0)
	}
}

func (u *ALU) runSMAXI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	src0i := emu.AsInt32(uint32(src0))
	src1i := emu.AsInt32(uint32(src1))
	if src0i > src1i {
		state.WriteOperand(inst.Dst, 0, uint64(emu.Int32ToBits(src0i)))
		state.SetSCC(1)
	} else {
		state.WriteOperand(inst.Dst, 0, uint64(emu.Int32ToBits(src1i)))
		state.SetSCC(0)
	}
}

func (u *ALU) runSMAXU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	if src0 > src1 {
		state.WriteOperand(inst.Dst, 0, src0)
		state.SetSCC(1)
	} else {
		state.WriteOperand(inst.Dst, 0, src1)
		state.SetSCC(0)
	}
}

func (u *ALU) runSCSELECTB32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	if state.SCC() == 1 {
		state.WriteOperand(inst.Dst, 0, src0)
	} else {
		state.WriteOperand(inst.Dst, 0, src1)
	}
}

func (u *ALU) runSCSELECTB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	if state.SCC() == 1 {
		state.WriteOperand(inst.Dst, 0, src0)
	} else {
		state.WriteOperand(inst.Dst, 0, src1)
	}
}

func (u *ALU) runSANDB32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	dst := src0 & src1
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSANDB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	dst := src0 & src1
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSORB32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	dst := uint64(uint32(src0) | uint32(src1))
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSORB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	dst := src0 | src1
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSXORB32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	dst := uint64(uint32(src0) ^ uint32(src1))
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSXORB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	dst := src0 ^ src1
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSANDN2B32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	dst := uint64(uint32(src0) & ^uint32(src1))
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSANDN2B64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	dst := src0 & ^src1
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSORN2B32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	dst := uint64(uint32(src0) | ^uint32(src1))
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSORN2B64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	dst := src0 | ^src1
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSLSHLB32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	shift := src1 & 0x1F
	dst := (src0 << shift) & 0xFFFFFFFF
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSLSHLB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	shift := src1 & 0x3F
	dst := src0 << shift
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSLSHRB32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	shift := src1 & 0x1F
	dst := uint64(uint32(src0) >> shift)
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSLSHRB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	shift := src1 & 0x3F
	dst := src0 >> shift
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSASHRI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	src1 := state.ReadOperand(inst.Src1, 0)
	shift := src1 & 0x1F
	result := src0 >> shift
	dst := uint64(emu.Int32ToBits(result))
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSASHRI64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := int64(state.ReadOperand(inst.Src0, 0))
	src1 := state.ReadOperand(inst.Src1, 0)
	shift := src1 & 0x3F
	dst := uint64(src0 >> shift)
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSBFMB32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	// S_BFM_B32: dst = ((1 << src0[4:0]) - 1) << src1[4:0]
	width := src0 & 0x1F
	offset := src1 & 0x1F
	mask := ((uint64(1) << width) - 1) << offset
	state.WriteOperand(inst.Dst, 0, mask&0xFFFFFFFF)
}

func (u *ALU) runSMULI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, 0)))
	result := src0 * src1
	state.WriteOperand(inst.Dst, 0, uint64(emu.Int32ToBits(result)))
}

func (u *ALU) runSBFEU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	offset := src1 & 0x1F
	width := (src1 >> 16) & 0x7F
	var dst uint64
	if width == 0 {
		dst = 0
	} else {
		dst = (src0 >> offset) & ((1 << width) - 1)
	}
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSMULHIU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	// S_MUL_HI_U32: D = (S0 * S1) >> 32
	result := src0 * src1
	state.WriteOperand(inst.Dst, 0, result>>32)
}

func (u *ALU) runSBFEI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	// S_BFE_I32: Extract bit field with sign extension
	offset := src1 & 0x1F
	width := (src1 >> 16) & 0x7F
	var dst uint64
	if width == 0 {
		dst = 0
	} else {
		extracted := (src0 >> offset) & ((1 << width) - 1)
		// Sign extend
		signBit := (extracted >> (width - 1)) & 1
		if signBit == 1 {
			signExt := ^((uint64(1) << width) - 1)
			extracted |= signExt
		}
		dst = extracted
	}
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}
