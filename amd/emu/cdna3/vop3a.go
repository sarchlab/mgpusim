package cdna3

import (
	"log"
	"math"
	"sort"
	"strings"

	"github.com/sarchlab/mgpusim/v4/amd/bitops"
	"github.com/sarchlab/mgpusim/v4/amd/emu"
)

//nolint:gocyclo,funlen
func (u *ALU) runVOP3A(state emu.InstEmuState) {
	inst := state.Inst()

	u.vop3aPreprocess(state)

	switch inst.Opcode {
	case 65: // 0x41
		u.runVCmpLtF32VOP3a(state)
	case 68: // 0x44
		u.runVCmpGtF32VOP3a(state)
	case 70: // 0x46
		u.runVCmpLeF32VOP3a(state)
	case 78: // 0x41
		u.runVCmpNltF32VOP3a(state)
	case 193: // 0xC1
		u.runVCmpLtI32VOP3a(state)
	case 195: // 0xC3
		u.runVCmpLeI32VOP3a(state)
	case 196: // 0xC4
		u.runVCmpGtI32VOP3a(state)
	case 198: // 0xC6
		u.runVCmpGEI32VOP3a(state)
	case 201: // 0xC9
		u.runVCmpLtU32VOP3a(state)
	case 202: // 0xCA
		u.runVCmpEqU32VOP3a(state)
	case 203: // 0xCB
		u.runVCmpLeU32VOP3a(state)
	case 204: // 0xCC
		u.runVCmpGtU32VOP3a(state)
	case 205: // 0xCD
		u.runVCmpLgU32VOP3a(state)
	case 206: // 0xCE
		u.runVCmpGeU32VOP3a(state)
	case 233: // 0xE9
		u.runVCmpLtU64VOP3a(state)
	case 256:
		u.runVCNDMASKB32VOP3a(state)
	case 258:
		u.runVSUBF32VOP3a(state)
	case 449:
		u.runVMADF32(state)
	case 450:
		u.runVMADI32I24(state)
	case 451, 488:
		u.runVMADU64U32(state)
	case 456: // CDNA3-specific: v_bfe_u32
		u.runVBFEU32(state)
	case 457: // v_bfe_i32
		u.runVBFEI32(state)
	case 459: // v_fma_f32
		u.runVFMAF32(state)
	case 460:
		u.runVFMAF64(state)
	case 464:
		u.runVMIN3F32(state)
	case 465:
		u.runVMIN3I32(state)
	case 466:
		u.runVMIN3U32(state)
	case 467:
		u.runVMAX3F32(state)
	case 468:
		u.runVMAX3I32(state)
	case 469:
		u.runVMAX3U32(state)
	case 470:
		u.runVMED3F32(state)
	case 471:
		u.runVMED3I32(state)
	case 472:
		u.runVMED3U32(state)
	case 478:
		u.runVDIVFIXUPF32(state)
	case 479:
		u.runVDIVFIXUPF64(state)
	case 482:
		u.runVDIVFMASF32(state)
	case 483:
		u.runVDIVFMASF64(state)
	case 640:
		u.runVADDF64(state)
	case 641:
		u.runVMULF64(state)
	case 645:
		u.runVMULLOU32(state)
	case 646:
		u.runVMULHIU32(state)
	case 944:
		u.runVPKFMAF32(state)
	case 945:
		u.runVPKMULF32(state)
	case 946:
		u.runVPKADDF32(state)
	case 509:
		u.runVLSHLADDU32(state)
	case 511:
		u.runVADD3U32(state)
	case 520:
		u.runVLSHLADDU64(state)
	case 655:
		u.runVLSHLREVB64(state)
	case 657:
		u.runVASHRREVI64(state)
	default:
		log.Panicf("Opcode %d for VOP3a format is not implemented", inst.Opcode)
	}
	u.vop3aPostprocess(state)
}

// runVBFEU32 implements v_bfe_u32 (Vector Bit Field Extract unsigned 32-bit)
// D.u = (S0.u >> S1.u[4:0]) & ((1 << S2.u[4:0]) - 1)
func (u *ALU) runVBFEU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := uint32(sp.SRC0[i])
		offset := uint32(sp.SRC1[i]) & 0x1F // [4:0] = 5 bits
		width := uint32(sp.SRC2[i]) & 0x1F  // [4:0] = 5 bits

		var result uint32
		if width == 0 {
			result = 0
		} else if width >= 32 {
			result = src0 >> offset
		} else {
			mask := (uint32(1) << width) - 1
			result = (src0 >> offset) & mask
		}

		sp.DST[i] = uint64(result)
	}
}

// runVBFEI32 implements v_bfe_i32 (Vector Bit Field Extract signed 32-bit)
// Similar to v_bfe_u32 but with sign extension
func (u *ALU) runVBFEI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := uint32(sp.SRC0[i])
		offset := uint32(sp.SRC1[i]) & 0x1F // [4:0] = 5 bits
		width := uint32(sp.SRC2[i]) & 0x1F  // [4:0] = 5 bits

		var result int32
		if width == 0 {
			result = 0
		} else {
			// Extract the field
			extracted := (src0 >> offset)
			if width < 32 {
				mask := (uint32(1) << width) - 1
				extracted &= mask
				// Sign extend
				signBit := (extracted >> (width - 1)) & 1
				if signBit == 1 {
					// Extend sign
					signExtMask := ^((uint32(1) << width) - 1)
					extracted |= signExtMask
				}
			}
			result = int32(extracted)
		}

		sp.DST[i] = uint64(emu.Int32ToBits(result))
	}
}

// runVLSHLADDU32 implements v_lshl_add_u32 (shift left and add 32-bit)
// D.u = (S0.u << S1.u[4:0]) + S2.u
func (u *ALU) runVLSHLADDU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := uint32(sp.SRC0[i])
		shift := uint32(sp.SRC1[i]) & 0x1F // [4:0] = 5 bits
		src2 := uint32(sp.SRC2[i])

		result := (src0 << shift) + src2
		sp.DST[i] = uint64(result)
	}
}

// runVADD3U32 implements v_add3_u32 (add three unsigned 32-bit values)
// D.u = S0.u + S1.u + S2.u
func (u *ALU) runVADD3U32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := uint32(sp.SRC0[i])
		src1 := uint32(sp.SRC1[i])
		src2 := uint32(sp.SRC2[i])

		result := src0 + src1 + src2
		sp.DST[i] = uint64(result)
	}
}

// runVLSHLADDU64 implements v_lshl_add_u64 (shift left and add 64-bit)
// D.u64 = (S0.u64 << S1.u[5:0]) + S2.u64
func (u *ALU) runVLSHLADDU64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		shift := sp.SRC1[i] & 0x3F // [5:0] = 6 bits for 64-bit shift
		src2 := sp.SRC2[i]

		result := (src0 << shift) + src2
		sp.DST[i] = result
	}
}

func (u *ALU) vop3aPreprocess(state emu.InstEmuState) {
	inst := state.Inst()

	if inst.Abs != 0 {
		u.vop3aPreProcessAbs(state)
	}

	if inst.Neg != 0 {
		u.vop3aPreProcessNeg(state)
	}
}

func (u *ALU) vop3aPreProcessAbs(state emu.InstEmuState) {
	inst := state.Inst()
	sp := state.Scratchpad().AsVOP3A()

	if strings.HasPrefix(inst.InstName, "v_pk_") {
		return
	}

	if strings.Contains(inst.InstName, "F32") ||
		strings.Contains(inst.InstName, "f32") {
		if inst.Abs&0x1 != 0 {
			for i := 0; i < 64; i++ {
				src0 := math.Float32frombits(uint32(sp.SRC0[i]))
				src0 = float32(math.Abs(float64(src0)))
				sp.SRC0[i] = uint64(math.Float32bits(src0))
			}
		}

		if inst.Abs&0x2 != 0 {
			for i := 0; i < 64; i++ {
				src1 := math.Float32frombits(uint32(sp.SRC1[i]))
				src1 = float32(math.Abs(float64(src1)))
				sp.SRC1[i] = uint64(math.Float32bits(src1))
			}
		}

		if inst.Abs&0x4 != 0 {
			for i := 0; i < 64; i++ {
				src2 := math.Float32frombits(uint32(sp.SRC2[i]))
				src2 = float32(math.Abs(float64(src2)))
				sp.SRC2[i] = uint64(math.Float32bits(src2))
			}
		}
	} else if strings.Contains(inst.InstName, "U32") ||
		strings.Contains(inst.InstName, "u32") ||
		strings.Contains(inst.InstName, "I32") ||
		strings.Contains(inst.InstName, "i32") ||
		strings.Contains(inst.InstName, "U64") ||
		strings.Contains(inst.InstName, "u64") ||
		strings.Contains(inst.InstName, "I24") ||
		strings.Contains(inst.InstName, "i24") {
		// Integer instructions: abs modifier is not applicable (used as opsel or unused).
		// The instruction still executes correctly without applying abs.
	} else {
		log.Printf("Absolute operation for %s is not implemented.", inst.InstName)
	}
}

func (u *ALU) vop3aPreProcessNeg(state emu.InstEmuState) {
	inst := state.Inst()

	if strings.HasPrefix(inst.InstName, "v_pk_") {
		return
	}

	if strings.Contains(inst.InstName, "F64") ||
		strings.Contains(inst.InstName, "f64") {
		u.vop3aPreProcessF64Neg(state)
	} else if strings.Contains(inst.InstName, "F32") ||
		strings.Contains(inst.InstName, "f32") {
		u.vop3aPreProcessF32Neg(state)
	} else if strings.Contains(inst.InstName, "B32") ||
		strings.Contains(inst.InstName, "b32") {
		u.vop3aPreProcessB32Neg(state)
	} else {
		log.Printf("Negative operation for %s is not implemented.", inst.InstName)
	}
}

func (u *ALU) vop3aPreProcessF64Neg(state emu.InstEmuState) {
	inst := state.Inst()
	sp := state.Scratchpad().AsVOP3A()

	if inst.Neg&0x1 != 0 {
		for i := 0; i < 64; i++ {
			src0 := math.Float64frombits(sp.SRC0[i])
			src0 = src0 * (-1.0)
			sp.SRC0[i] = math.Float64bits(src0)
		}
	}

	if inst.Neg&0x2 != 0 {
		for i := 0; i < 64; i++ {
			src1 := math.Float64frombits(sp.SRC1[i])
			src1 = src1 * (-1.0)
			sp.SRC1[i] = math.Float64bits(src1)
		}
	}

	if inst.Neg&0x4 != 0 {
		for i := 0; i < 64; i++ {
			src2 := math.Float64frombits(sp.SRC2[i])
			src2 = src2 * (-1.0)
			sp.SRC2[i] = math.Float64bits(src2)
		}
	}
}

func (u *ALU) vop3aPreProcessF32Neg(state emu.InstEmuState) {
	inst := state.Inst()
	sp := state.Scratchpad().AsVOP3A()
	if inst.Neg&0x1 != 0 {
		for i := 0; i < 64; i++ {
			src0 := math.Float32frombits(uint32(sp.SRC0[i]))
			src0 = src0 * (-1.0)
			sp.SRC0[i] = uint64(math.Float32bits(src0))
		}
	}

	if inst.Neg&0x2 != 0 {
		for i := 0; i < 64; i++ {
			src1 := math.Float32frombits(uint32(sp.SRC1[i]))
			src1 = src1 * (-1.0)
			sp.SRC1[i] = uint64(math.Float32bits(src1))
		}
	}

	if inst.Neg&0x4 != 0 {
		for i := 0; i < 64; i++ {
			src2 := math.Float32frombits(uint32(sp.SRC2[i]))
			src2 = src2 * (-1.0)
			sp.SRC2[i] = uint64(math.Float32bits(src2))
		}
	}
}

func (u *ALU) vop3aPreProcessB32Neg(state emu.InstEmuState) {
	inst := state.Inst()
	sp := state.Scratchpad().AsVOP3A()
	if inst.Neg&0x1 != 0 {
		for i := 0; i < 64; i++ {
			src0 := emu.AsInt32(uint32(sp.SRC0[i]))
			src0 = src0 * (-1.0)
			sp.SRC0[i] = uint64(emu.Int32ToBits(src0))
		}
	}

	if inst.Neg&0x2 != 0 {
		for i := 0; i < 64; i++ {
			src1 := emu.AsInt32(uint32(sp.SRC1[i]))
			src1 = src1 * (-1.0)
			sp.SRC1[i] = uint64(emu.Int32ToBits(src1))
		}
	}

	if inst.Neg&0x4 != 0 {
		for i := 0; i < 64; i++ {
			src2 := emu.AsInt32(uint32(sp.SRC2[i]))
			src2 = src2 * (-1.0)
			sp.SRC2[i] = uint64(emu.Int32ToBits(src2))
		}
	}
}

func (u *ALU) vop3aPostprocess(state emu.InstEmuState) {
	inst := state.Inst()

	if strings.HasPrefix(inst.InstName, "v_pk_") {
		return
	}

	if inst.Omod != 0 {
		log.Panic("Output modifiers are not supported.")
	}
}

func (u *ALU) runVCmpLtF32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	var i uint
	var src0, src1 float32
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 = math.Float32frombits(uint32(sp.SRC0[i]))
		src1 = math.Float32frombits(uint32(sp.SRC1[i]))
		if src0 < src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpGtF32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	var i uint
	var src0, src1 float32
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 = math.Float32frombits(uint32(sp.SRC0[i]))
		src1 = math.Float32frombits(uint32(sp.SRC1[i]))
		if src0 > src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLeF32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	var i uint
	var src0, src1 float32
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 = math.Float32frombits(uint32(sp.SRC0[i]))
		src1 = math.Float32frombits(uint32(sp.SRC1[i]))
		if src0 <= src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpNltF32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	var i uint
	var src0, src1 float32
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 = math.Float32frombits(uint32(sp.SRC0[i]))
		src1 = math.Float32frombits(uint32(sp.SRC1[i]))
		if !(src0 < src1) {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLtI32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := emu.AsInt32(uint32(sp.SRC0[i]))
		src1 := emu.AsInt32(uint32(sp.SRC1[i]))

		if src0 < src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLeI32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := emu.AsInt32(uint32(sp.SRC0[i]))
		src1 := emu.AsInt32(uint32(sp.SRC1[i]))

		if src0 <= src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpGtI32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := emu.AsInt32(uint32(sp.SRC0[i]))
		src1 := emu.AsInt32(uint32(sp.SRC1[i]))

		if src0 > src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpGEI32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := emu.AsInt32(uint32(sp.SRC0[i]))
		src1 := emu.AsInt32(uint32(sp.SRC1[i]))

		if src0 >= src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLtU32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 < src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpEqU32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if uint32(src0) == uint32(src1) {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLeU32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 <= src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpGtU32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 > src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLgU32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 != src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpGeU32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 >= src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLtU64VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 < src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCNDMASKB32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		if (sp.SRC2[i] & (1 << i)) > 0 {
			sp.DST[i] = sp.SRC1[i]
		} else {
			sp.DST[i] = sp.SRC0[i]
		}
	}
}

func (u *ALU) runVSUBF32VOP3a(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		dst := src0 - src1
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVMADF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		src2 := math.Float32frombits(uint32(sp.SRC2[i]))

		res := src0*src1 + src2
		sp.DST[i] = uint64(math.Float32bits(res))
	}
}

func (u *ALU) runVMADI32I24(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := int32(bitops.SignExt(
			bitops.ExtractBitsFromU64(sp.SRC0[i], 0, 23), 23))
		src1 := int32(bitops.SignExt(
			bitops.ExtractBitsFromU64(sp.SRC1[i], 0, 23), 23))
		src2 := int32(sp.SRC2[i])

		sp.DST[i] = uint64(src0*src1 + src2)
	}
}

func (u *ALU) runVMADU64U32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = sp.SRC0[i]*sp.SRC1[i] + sp.SRC2[i]
	}
}

func (u *ALU) runVMULLOU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = (sp.SRC0[i] * sp.SRC1[i])
	}
}

func (u *ALU) runVMULHIU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = (sp.SRC0[i] * sp.SRC1[i]) >> 32
	}
}

func (u *ALU) runVLSHLREVB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		shift := sp.SRC0[i]
		src := sp.SRC1[i]
		result := src << shift
		sp.DST[i] = result
	}
}

func (u *ALU) runVASHRREVI64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = emu.Int64ToBits(emu.AsInt64(sp.SRC1[i]) >> sp.SRC0[i])
	}
}

func (u *ALU) runVADDF64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	if !inst.IsSdwa {
		var i uint
		for i = 0; i < 64; i++ {
			if !emu.LaneMasked(sp.EXEC, i) {
				continue
			}

			src0 := math.Float64frombits(sp.SRC0[i])
			src1 := math.Float64frombits(sp.SRC1[i])
			dst := src0 + src1
			sp.DST[i] = math.Float64bits(dst)
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func (u *ALU) runVFMAF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		src2 := math.Float32frombits(uint32(sp.SRC2[i]))

		// v_fma_f32: D.f = S0.f * S1.f + S2.f (fused multiply-add)
		dst := src0*src1 + src2
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVFMAF64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	if !inst.IsSdwa {
		var i uint
		for i = 0; i < 64; i++ {
			if !emu.LaneMasked(sp.EXEC, i) {
				continue
			}
			src0 := math.Float64frombits(sp.SRC0[i])
			src1 := math.Float64frombits(sp.SRC1[i])
			src2 := math.Float64frombits(sp.SRC2[i])

			dst := src0*src1 + src2
			sp.DST[i] = math.Float64bits(dst)
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func (u *ALU) runVMIN3F32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	if !inst.IsSdwa {
		var i uint
		for i = 0; i < 64; i++ {
			if !emu.LaneMasked(sp.EXEC, i) {
				continue
			}

			src0 := math.Float32frombits(uint32(sp.SRC0[i]))
			src1 := math.Float32frombits(uint32(sp.SRC1[i]))
			src2 := math.Float32frombits(uint32(sp.SRC2[i]))

			dst := src0
			if src1 < dst {
				dst = src1
			}
			if src2 < dst {
				dst = src2
			}

			sp.DST[i] = uint64(math.Float32bits(dst))
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func (u *ALU) runVMIN3I32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	if !inst.IsSdwa {
		var i uint
		for i = 0; i < 64; i++ {
			if !emu.LaneMasked(sp.EXEC, i) {
				continue
			}

			src0 := emu.AsInt32(uint32(sp.SRC0[i]))
			src1 := emu.AsInt32(uint32(sp.SRC1[i]))
			src2 := emu.AsInt32(uint32(sp.SRC2[i]))

			dst := src0
			if src1 < dst {
				dst = src1
			}
			if src2 < dst {
				dst = src2
			}

			sp.DST[i] = uint64(emu.Int32ToBits(dst))
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func (u *ALU) runVMIN3U32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	if !inst.IsSdwa {
		var i uint
		for i = 0; i < 64; i++ {
			if !emu.LaneMasked(sp.EXEC, i) {
				continue
			}

			src0 := uint32(sp.SRC0[i])
			src1 := uint32(sp.SRC1[i])
			src2 := uint32(sp.SRC2[i])

			dst := src0
			if src1 < dst {
				dst = src1
			}
			if src2 < dst {
				dst = src2
			}

			sp.DST[i] = uint64(dst)
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func (u *ALU) runVMAX3F32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	if !inst.IsSdwa {
		var i uint
		for i = 0; i < 64; i++ {
			if !emu.LaneMasked(sp.EXEC, i) {
				continue
			}

			src0 := math.Float32frombits(uint32(sp.SRC0[i]))
			src1 := math.Float32frombits(uint32(sp.SRC1[i]))
			src2 := math.Float32frombits(uint32(sp.SRC2[i]))

			dst := src0
			if src1 > dst {
				dst = src1
			}
			if src2 > dst {
				dst = src2
			}

			sp.DST[i] = uint64(math.Float32bits(dst))
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func (u *ALU) runVMAX3I32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	if !inst.IsSdwa {
		var i uint
		for i = 0; i < 64; i++ {
			if !emu.LaneMasked(sp.EXEC, i) {
				continue
			}

			src0 := emu.AsInt32(uint32(sp.SRC0[i]))
			src1 := emu.AsInt32(uint32(sp.SRC1[i]))
			src2 := emu.AsInt32(uint32(sp.SRC2[i]))

			dst := src0
			if src1 > dst {
				dst = src1
			}
			if src2 > dst {
				dst = src2
			}

			sp.DST[i] = uint64(emu.Int32ToBits(dst))
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func (u *ALU) runVMAX3U32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	if !inst.IsSdwa {
		var i uint
		for i = 0; i < 64; i++ {
			if !emu.LaneMasked(sp.EXEC, i) {
				continue
			}

			src0 := uint32(sp.SRC0[i])
			src1 := uint32(sp.SRC1[i])
			src2 := uint32(sp.SRC2[i])

			dst := src0
			if src1 > dst {
				dst = src1
			}
			if src2 > dst {
				dst = src2
			}

			sp.DST[i] = uint64(dst)
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func (u *ALU) runVMED3F32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	if !inst.IsSdwa {
		var i uint
		for i = 0; i < 64; i++ {
			if !emu.LaneMasked(sp.EXEC, i) {
				continue
			}

			src0 := math.Float32frombits(uint32(sp.SRC0[i]))
			src1 := math.Float32frombits(uint32(sp.SRC1[i]))
			src2 := math.Float32frombits(uint32(sp.SRC2[i]))

			list := []float64{float64(src0), float64(src1), float64(src2)}
			sort.Float64s(list)

			sp.DST[i] = uint64(math.Float32bits(float32(list[1])))
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func (u *ALU) runVMED3I32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	if !inst.IsSdwa {
		var i uint
		for i = 0; i < 64; i++ {
			if !emu.LaneMasked(sp.EXEC, i) {
				continue
			}

			src0 := emu.AsInt32(uint32(sp.SRC0[i]))
			src1 := emu.AsInt32(uint32(sp.SRC1[i]))
			src2 := emu.AsInt32(uint32(sp.SRC2[i]))

			list := []int{int(src0), int(src1), int(src2)}
			sort.Ints(list)

			dst := int32(list[1])
			sp.DST[i] = uint64(emu.Int32ToBits(dst))
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func (u *ALU) runVMED3U32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	if !inst.IsSdwa {
		var i uint
		for i = 0; i < 64; i++ {
			if !emu.LaneMasked(sp.EXEC, i) {
				continue
			}

			src0 := uint32(sp.SRC0[i])
			src1 := uint32(sp.SRC1[i])
			src2 := uint32(sp.SRC2[i])

			dst := median3Uint32(src0, src1, src2)
			sp.DST[i] = uint64(dst)
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func median3Uint32(a, b, c uint32) uint32 {
	out := a

	if (b < a && b > c) || (b > a && b < c) {
		out = b
	}

	if (c < a && c > b) || (c > a && c < b) {
		out = c
	}

	return out
}

func (u *ALU) runVMULF64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	if !inst.IsSdwa {
		var i uint
		for i = 0; i < 64; i++ {
			if !emu.LaneMasked(sp.EXEC, i) {
				continue
			}
			src0 := math.Float64frombits(sp.SRC0[i])
			src1 := math.Float64frombits(sp.SRC1[i])

			dst := src0 * src1
			sp.DST[i] = math.Float64bits(dst)
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func (u *ALU) runVDIVFMASF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		vccBit := (sp.VCC >> i) & 1

		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		src2 := math.Float32frombits(uint32(sp.SRC2[i]))

		// v_div_fmas_f32: Part of software division - final step
		// Simplified: if VCC[i], scale by 2^32, else normal FMA
		var dst float32
		if vccBit == 1 {
			dst = float32(math.Pow(2.0, 32)) * (src0*src1 + src2)
		} else {
			dst = src0*src1 + src2
		}
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVDIVFMASF64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()

	if !inst.IsSdwa {
		var i uint
		for i = 0; i < 64; i++ {
			if !emu.LaneMasked(sp.EXEC, i) {
				continue
			}

			vccVal := (sp.VCC) & (1 << i)

			src0 := math.Float64frombits(sp.SRC0[i])
			src1 := math.Float64frombits(sp.SRC1[i])
			src2 := math.Float64frombits(sp.SRC2[i])

			var dst float64
			if vccVal == 1 {
				dst = math.Pow(2.0, 64) * (src0*src1 + src2)
			} else {
				dst = src0*src1 + src2
			}
			sp.DST[i] = math.Float64bits(dst)
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func (u *ALU) runVDIVFIXUPF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		src2 := math.Float32frombits(uint32(sp.SRC2[i]))

		// v_div_fixup_f32: Final fixup for division
		// Simplified: handles special cases (NaN, inf, denormals)
		// For normal values, just use src0 as the quotient
		dst := src0
		
		// Handle special cases
		if math.IsNaN(float64(src1)) || math.IsNaN(float64(src2)) {
			dst = float32(math.NaN())
		} else if math.IsInf(float64(src1), 0) && math.IsInf(float64(src2), 0) {
			dst = float32(math.NaN())
		} else if src2 == 0 && src1 != 0 {
			// Division by zero
			if src1 > 0 {
				dst = float32(math.Inf(1))
			} else {
				dst = float32(math.Inf(-1))
			}
		}
		
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVDIVFIXUPF64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()

	if inst.IsSdwa {
		log.Panicf("SDWA for VOP3A instruction opcode %d not implemented \n", inst.Opcode)
	}

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = u.calculateDivFixUpF64(
			sp.SRC0[i], sp.SRC1[i], sp.SRC2[i])
	}
}

//nolint:gocyclo,funlen
func (u *ALU) calculateDivFixUpF64(
	src0Bits, src1Bits, src2Bits uint64,
) uint64 {
	signS1 := src1Bits >> 63
	signS2 := src2Bits >> 63
	signOut := (signS1) ^ (signS2)

	src0 := math.Float64frombits(src0Bits)
	src1 := math.Float64frombits(src1Bits)
	src2 := math.Float64frombits(src2Bits)

	exponentSrc1 := (src1Bits << 1) >> 53
	exponentSrc2 := (src2Bits << 1) >> 53

	var dst float64

	nan := math.Float64frombits(0x7FFFFFFFFFFFFFFF)
	nanWithQuieting := math.Float64frombits(0x7FF8_0000_0000_0001)
	undetermined := float64(0xFFF8_0000_0000_0000)

	if src2 == nan {
		dst = nanWithQuieting
	} else if src1 == nan {
		dst = nanWithQuieting
	} else if (src1 == 0) && (src2 == 0) {
		dst = undetermined
	} else if u.isInfByInf(src1, src2) {
		dst = undetermined
	} else if src1 == 0 || (math.Abs(src2) == 0x7FF0000000000000 || math.Abs(src2) == 0xFFF0000000000000) {
		// x/0 , or inf / y
		if signOut == 1 {
			dst = 0xFFF0000000000000 // -INF
		} else {
			dst = 0x7FF0000000000000 // +INF
		}
	} else if (math.Abs(src1) == 0x7FF0000000000000 || math.Abs(src1) == 0xFFF0000000000000) || (src2 == 0) {
		// x/inf, 0/y
		if signOut == 1 {
			dst = 0x8000000000000000 // -0
		} else {
			dst = 0x0000000000000000 // +0
		}
	} else if u.isDIVFIXUPF64Overflow(exponentSrc1, exponentSrc2) {
		log.Panicf("Underflow for VOP3A instruction DIVFIXUPF64 not implemented \n")
	} else {
		if signOut == 1 {
			dst = math.Abs(src0) * (-1.0)
		} else {
			dst = math.Abs(src0)
		}
	}

	return math.Float64bits(dst)
}

func (u *ALU) isInfByInf(src1, src2 float64) bool {
	return (math.Abs(src1) == 0x7FF0000000000000 ||
		math.Abs(src1) == 0xFFF0000000000000) &&
		(math.Abs(src2) == 0x7FF0000000000000 ||
			math.Abs(src2) == 0xFFF0000000000000)
}

func (u *ALU) isDIVFIXUPF64Overflow(
	exponentSrc1, exponentSrc2 uint64,
) bool {
	return int64(exponentSrc2-exponentSrc1) < -1075 ||
		exponentSrc1 == 2047
}

func (u *ALU) runVPKFMAF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	
	// VOP3P encoding: OpSel (bits 11-14) and OpSelHi (bits 59-60)
	op_sel := inst.OpSel
	op_sel_hi := inst.OpSelHi

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0Bits := sp.SRC0[i]
		src1Bits := sp.SRC1[i]
		src2Bits := sp.SRC2[i]

		src0_lo := math.Float32frombits(uint32(src0Bits))
		src0_hi := math.Float32frombits(uint32(src0Bits >> 32))
		src1_lo := math.Float32frombits(uint32(src1Bits))
		src1_hi := math.Float32frombits(uint32(src1Bits >> 32))
		src2_lo := math.Float32frombits(uint32(src2Bits))
		src2_hi := math.Float32frombits(uint32(src2Bits >> 32))

		var a_lo, a_hi float32
		var b_lo, b_hi float32
		var c_lo, c_hi float32

		if op_sel&1 != 0 { a_lo = src0_hi } else { a_lo = src0_lo }
		if op_sel&2 != 0 { b_lo = src1_hi } else { b_lo = src1_lo }
		if op_sel&4 != 0 { c_lo = src2_hi } else { c_lo = src2_lo }

		if op_sel_hi&1 != 0 { a_hi = src0_hi } else { a_hi = src0_lo }
		if op_sel_hi&2 != 0 { b_hi = src1_hi } else { b_hi = src1_lo }
		if op_sel_hi&4 != 0 { c_hi = src2_hi } else { c_hi = src2_lo }

		res_lo := a_lo * b_lo + c_lo
		res_hi := a_hi * b_hi + c_hi

		dstBits := uint64(math.Float32bits(res_lo)) | (uint64(math.Float32bits(res_hi)) << 32)
		sp.DST[i] = dstBits
	}
}

func (u *ALU) runVPKMULF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	
	// VOP3P encoding for 2-source packed f32 operations:
	// OpSel (bits 11-14, 4 bits total) controls ALL source selection:
	// OpSel[0] (bit 0): src0 word select for lower result (0=lo, 1=hi)
	// OpSel[1] (bit 1): src1 word select for lower result (0=lo, 1=hi)
	// OpSel[2] (bit 2): src0 word select for upper result (0=lo, 1=hi)
	// OpSel[3] (bit 3): src1 word select for upper result (0=lo, 1=hi)
	// Note: OpSelHi is NOT used for 2-source packed ops, only for 3-source FMA
	op_sel := inst.OpSel

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0Bits := sp.SRC0[i]
		src1Bits := sp.SRC1[i]

		src0_lo := math.Float32frombits(uint32(src0Bits))
		src0_hi := math.Float32frombits(uint32(src0Bits >> 32))
		src1_lo := math.Float32frombits(uint32(src1Bits))
		src1_hi := math.Float32frombits(uint32(src1Bits >> 32))

		var a_lo, b_lo float32  // Inputs for lower result
		var a_hi, b_hi float32  // Inputs for upper result

		// Lower result: OpSel[0] and OpSel[1]
		if op_sel&0b0001 != 0 { a_lo = src0_hi } else { a_lo = src0_lo }
		if op_sel&0b0010 != 0 { b_lo = src1_hi } else { b_lo = src1_lo }

		// Upper result: OpSel[2] and OpSel[3]
		if op_sel&0b0100 != 0 { a_hi = src0_hi } else { a_hi = src0_lo }
		if op_sel&0b1000 != 0 { b_hi = src1_hi } else { b_hi = src1_lo }

		res_lo := a_lo * b_lo
		res_hi := a_hi * b_hi

		dstBits := uint64(math.Float32bits(res_lo)) | (uint64(math.Float32bits(res_hi)) << 32)
		sp.DST[i] = dstBits
	}
}

func (u *ALU) runVPKADDF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	
	// VOP3P encoding for 2-source packed f32 operations:
	// OpSel (bits 11-14, 4 bits total) controls ALL source selection:
	// OpSel[0] (bit 0): src0 word select for lower result (0=lo, 1=hi)
	// OpSel[1] (bit 1): src1 word select for lower result (0=lo, 1=hi)
	// OpSel[2] (bit 2): src0 word select for upper result (0=lo, 1=hi)
	// OpSel[3] (bit 3): src1 word select for upper result (0=lo, 1=hi)
	// Note: OpSelHi is NOT used for 2-source packed ops, only for 3-source FMA
	op_sel := inst.OpSel

	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}

		src0Bits := sp.SRC0[i]
		src1Bits := sp.SRC1[i]

		src0_lo := math.Float32frombits(uint32(src0Bits))
		src0_hi := math.Float32frombits(uint32(src0Bits >> 32))
		src1_lo := math.Float32frombits(uint32(src1Bits))
		src1_hi := math.Float32frombits(uint32(src1Bits >> 32))

		var a_lo, a_hi float32
		var b_lo, b_hi float32

		// Lower result: OpSel[0] and OpSel[1]
		if op_sel&0b0001 != 0 { a_lo = src0_hi } else { a_lo = src0_lo }
		if op_sel&0b0010 != 0 { b_lo = src1_hi } else { b_lo = src1_lo }

		// Upper result: OpSel[2] and OpSel[3]
		if op_sel&0b0100 != 0 { a_hi = src0_hi } else { a_hi = src0_lo }
		if op_sel&0b1000 != 0 { b_hi = src1_hi } else { b_hi = src1_lo }

		res_lo := a_lo + b_lo
		res_hi := a_hi + b_hi

		dstBits := uint64(math.Float32bits(res_lo)) | (uint64(math.Float32bits(res_hi)) << 32)
		sp.DST[i] = dstBits
	}
}
