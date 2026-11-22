package cdna3

import (
	"log"
	"math"
	"sort"
	"strings"

	"github.com/sarchlab/mgpusim/v5/amd/bitops"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

func applyF32Modifier(val uint64, srcIdx int, inst *insts.Inst) uint64 {
	f := math.Float32frombits(uint32(val))
	if inst.Abs&(1<<uint(srcIdx)) != 0 {
		f = float32(math.Abs(float64(f)))
	}
	if inst.Neg&(1<<uint(srcIdx)) != 0 {
		f = -f
	}
	return uint64(math.Float32bits(f))
}

func applyF64Modifier(val uint64, srcIdx int, inst *insts.Inst) uint64 {
	f := math.Float64frombits(val)
	if inst.Abs&(1<<uint(srcIdx)) != 0 {
		f = math.Abs(f)
	}
	if inst.Neg&(1<<uint(srcIdx)) != 0 {
		f = -f
	}
	return math.Float64bits(f)
}

func readModifiedF64(state emu.InstEmuState, operand *insts.Operand, lane, source int) uint64 {
	bits := math.Float64bits(readF64Operand(state, operand, lane))
	return applyF64Modifier(bits, source, state.Inst())
}

func applyB32Modifier(val uint64, srcIdx int, inst *insts.Inst) uint64 {
	if inst.Neg&(1<<uint(srcIdx)) != 0 {
		v := emu.AsInt32(uint32(val))
		v = -v
		return uint64(emu.Int32ToBits(v))
	}
	return val
}

//nolint:gocyclo,funlen
func (u *ALU) runVOP3A(state emu.InstEmuState) {
	inst := state.Inst()

	u.vop3aPreprocess(state)

	switch inst.Opcode {
	case 107, 110:
		u.runVCmpUnorderedF64(state, inst.Opcode == 110, inst.Dst)
	case 65: // 0x41 v_cmp_lt_f32
		u.runVCmpLtF32VOP3a(state)
	case 66: // 0x42 v_cmp_eq_f32
		u.runVCmpEqF32VOP3a(state)
	case 67: // 0x43 v_cmp_le_f32
		u.runVCmpLeF32VOP3a(state)
	case 68: // 0x44 v_cmp_gt_f32
		u.runVCmpGtF32VOP3a(state)
	case 69: // 0x45 v_cmp_lg_f32
		u.runVCmpLgF32VOP3a(state)
	case 70: // 0x46 v_cmp_ge_f32
		u.runVCmpGeF32VOP3a(state)
	case 71: // 0x47 v_cmp_o_f32
		u.runVCmpOF32VOP3a(state)
	case 72: // 0x48 v_cmp_u_f32
		u.runVCmpUF32VOP3a(state)
	case 73: // 0x49 v_cmp_nge_f32
		u.runVCmpNgeF32VOP3a(state)
	case 74: // 0x4a v_cmp_nlg_f32
		u.runVCmpNlgF32VOP3a(state)
	case 75: // 0x4b v_cmp_ngt_f32
		u.runVCmpNgtF32VOP3a(state)
	case 76: // 0x4c v_cmp_nle_f32
		u.runVCmpNleF32VOP3a(state)
	case 77: // 0x4d v_cmp_neq_f32
		u.runVCmpNeqF32VOP3a(state)
	case 16: // v_cmp_class_f32
		u.runVCmpClassF32VOP3a(state)
	case 78: // 0x4e v_cmp_nlt_f32
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
	case 257:
		u.runVADDF32VOP3a(state)
	case 258:
		u.runVSUBF32VOP3a(state)
	case 261:
		u.runVMULF32VOP3a(state)
	case 309: // v_sub_u32 (VOP3a clamp form)
		u.runVSUBU32VOP3a(state)
	case 449:
		u.runVMADF32(state)
	case 450:
		u.runVMADI32I24(state)
	case 451, 488:
		u.runVMADU64U32(state)
	case 489: // v_mad_i64_i32
		u.runVMADI64I32(state)
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
	case 510:
		u.runVADDLSHLU32(state)
	case 511:
		u.runVADD3U32(state)
	case 512:
		u.runVLSHLORB32(state)
	case 458:
		u.runVBFIB32(state)
	case 947:
		u.runVPKMOVB32(state)
	case 328:
		u.runVCVTI32F32(state)
	case 499:
		u.runVXADU32(state)
	case 644:
		u.runLDEXPF64(state)
	case 372:
		u.runFrexpF32(state, false)
	case 513:
		u.runVANDORB32(state)
	case 514: // v_or3_b32
		u.runVOR3B32(state)
	case 648: // v_ldexp_f32
		u.runVLDEXPF32(state)
	case 672:
		u.runPackB32F16(state)
	case 910, 911:
		u.runPackedF16(state)
	case 929, 930:
		u.runMixedF16(state)
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
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		offset := uint32(state.ReadOperand(inst.Src1, i)) & 0x1F
		width := uint32(state.ReadOperand(inst.Src2, i)) & 0x1F
		var result uint32
		if width == 0 {
			result = 0
		} else if width >= 32 {
			result = src0 >> offset
		} else {
			mask := (uint32(1) << width) - 1
			result = (src0 >> offset) & mask
		}
		state.WriteOperand(inst.Dst, i, uint64(result))
	}
}

// runVBFEI32 implements v_bfe_i32 (Vector Bit Field Extract signed 32-bit)
// Similar to v_bfe_u32 but with sign extension
func (u *ALU) runVBFEI32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		offset := uint32(state.ReadOperand(inst.Src1, i)) & 0x1F
		width := uint32(state.ReadOperand(inst.Src2, i)) & 0x1F
		var result int32
		if width == 0 {
			result = 0
		} else {
			extracted := (src0 >> offset)
			if width < 32 {
				mask := (uint32(1) << width) - 1
				extracted &= mask
				signBit := (extracted >> (width - 1)) & 1
				if signBit == 1 {
					signExtMask := ^((uint32(1) << width) - 1)
					extracted |= signExtMask
				}
			}
			result = int32(extracted)
		}
		state.WriteOperand(inst.Dst, i, uint64(emu.Int32ToBits(result)))
	}
}

// runVLSHLADDU32 implements v_lshl_add_u32 (shift left and add 32-bit)
// D.u = (S0.u << S1.u[4:0]) + S2.u
func (u *ALU) runVLSHLADDU32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		shift := uint32(state.ReadOperand(inst.Src1, i)) & 0x1F
		src2 := uint32(state.ReadOperand(inst.Src2, i))
		result := (src0 << shift) + src2
		state.WriteOperand(inst.Dst, i, uint64(result))
	}
}

// runVLSHLORB32 implements v_lshl_or_b32 (shift left and OR 32-bit)
// D.u = (S0.u << S1.u[4:0]) | S2.u
func (u *ALU) runVLSHLORB32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		shift := uint32(state.ReadOperand(inst.Src1, i)) & 0x1F
		src2 := uint32(state.ReadOperand(inst.Src2, i))
		result := (src0 << shift) | src2
		state.WriteOperand(inst.Dst, i, uint64(result))
	}
}

// runVADDLSHLU32 implements v_add_lshl_u32 (add and shift left 32-bit)
// D.u = (S0.u + S1.u) << S2.u[4:0]
func (u *ALU) runVADDLSHLU32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		shift := uint32(state.ReadOperand(inst.Src2, i)) & 0x1F
		result := (src0 + src1) << shift
		state.WriteOperand(inst.Dst, i, uint64(result))
	}
}

// runVADD3U32 implements v_add3_u32 (add three unsigned 32-bit values)
// D.u = S0.u + S1.u + S2.u
func (u *ALU) runVADD3U32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		src2 := uint32(state.ReadOperand(inst.Src2, i))
		result := src0 + src1 + src2
		state.WriteOperand(inst.Dst, i, uint64(result))
	}
}

// runVLSHLADDU64 implements v_lshl_add_u64 (shift left and add 64-bit)
// D.u64 = (S0.u64 << S1.u[5:0]) + S2.u64
func (u *ALU) runVLSHLADDU64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		shift := state.ReadOperand(inst.Src1, i) & 0x3F
		src2 := state.ReadOperand(inst.Src2, i)
		result := (src0 << shift) + src2
		state.WriteOperand(inst.Dst, i, result)
	}
}

func (u *ALU) vop3aPreprocess(state emu.InstEmuState) {
	// Abs/neg modifiers are now applied inline via helper functions.
}

func (u *ALU) vop3aPostprocess(state emu.InstEmuState) {
	inst := state.Inst()

	if strings.HasPrefix(inst.InstName, "v_pk_") || inst.Opcode == 929 || inst.Opcode == 930 {
		return
	}

	if inst.Omod != 0 {
		log.Panic("Output modifiers are not supported.")
	}
}

func (u *ALU) runVCmpLtF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		if src0 < src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpGtF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		if src0 > src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpLeF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		if src0 <= src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func matchesF32Class(src0 float32, classMask uint32) bool {
	bits := math.Float32bits(src0)
	sign := (bits >> 31) != 0
	exp := (bits >> 23) & 0xFF
	frac := bits & 0x7FFFFF
	isNaN := math.IsNaN(float64(src0))
	isInf := math.IsInf(float64(src0), 0)
	isDenorm := exp == 0 && frac != 0
	isZero := exp == 0 && frac == 0
	isNorm := !isNaN && !isInf && !isDenorm && !isZero

	classChecks := [10]bool{
		isNaN && frac&(1<<22) == 0, // 0: signaling NaN
		isNaN && frac&(1<<22) != 0, // 1: quiet NaN
		isInf && sign,              // 2: negative infinity
		isNorm && sign,             // 3: negative normal
		isDenorm && sign,           // 4: negative denormal
		isZero && sign,             // 5: negative zero
		isZero && !sign,            // 6: positive zero
		isDenorm && !sign,          // 7: positive denormal
		isNorm && !sign,            // 8: positive normal
		isInf && !sign,             // 9: positive infinity
	}
	for bit, check := range classChecks {
		if classMask&(1<<uint(bit)) != 0 && check {
			return true
		}
	}
	return false
}

func (u *ALU) runVCmpClassF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		classMask := uint32(state.ReadOperand(inst.Src1, i))
		if matchesF32Class(src0, classMask) {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpNltF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		if !(src0 < src1) {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpEqF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		if src0 == src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpLgF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		if src0 < src1 || src0 > src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpGeF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		if src0 >= src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpOF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		if !math.IsNaN(float64(src0)) && !math.IsNaN(float64(src1)) {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpUF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		if math.IsNaN(float64(src0)) || math.IsNaN(float64(src1)) {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpNgeF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		if !(src0 >= src1) {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpNlgF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		if !(src0 != src1) {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpNgtF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		if !(src0 > src1) {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpNleF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		if !(src0 <= src1) {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpNeqF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		if !(src0 == src1) {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpLtI32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 < src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpLeI32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 <= src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpGtI32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 > src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpGEI32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 >= src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpLtU32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		if src0 < src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpEqU32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		if src0 == src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpLeU32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		if src0 <= src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpGtU32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		if src0 > src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpLgU32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		if src0 != src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpGeU32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		if src0 >= src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCmpLtU64VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 < src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runVCNDMASKB32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		src2 := state.ReadOperand(inst.Src2, i)
		if (src2 & (1 << uint(i))) > 0 {
			state.WriteOperand(inst.Dst, i, src1)
		} else {
			state.WriteOperand(inst.Dst, i, src0)
		}
	}
}

func (u *ALU) runVADDF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		dst := src0 + src1
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runVSUBF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		dst := src0 - src1
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

// runVSUBU32VOP3a implements the VOP3A form of v_sub_u32.
// D.u = S0.u - S1.u. When the CLAMP bit is set, an unsigned underflow
// saturates the result to 0.
func (u *ALU) runVSUBU32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		result := src0 - src1
		if inst.Clamp && src1 > src0 {
			result = 0
		}
		state.WriteOperand(inst.Dst, i, uint64(result))
	}
}

// runVMADI64I32 implements v_mad_i64_i32 (signed).
// D.i64 = sext(S0.i32) * sext(S1.i32) + S2.i64
func (u *ALU) runVMADI64I32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := int64(emu.AsInt32(uint32(state.ReadOperand(inst.Src0, i))))
		src1 := int64(emu.AsInt32(uint32(state.ReadOperand(inst.Src1, i))))
		src2 := emu.AsInt64(state.ReadOperand(inst.Src2, i))
		result := src0*src1 + src2
		state.WriteOperand(inst.Dst, i, emu.Int64ToBits(result))
	}
}

// runVOR3B32 implements v_or3_b32 (bitwise OR of three 32-bit values).
// D.u = S0.u | S1.u | S2.u
func (u *ALU) runVBFIB32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for lane := 0; lane < 64; lane++ {
		if exec&(1<<uint(lane)) == 0 {
			continue
		}
		mask := uint32(state.ReadOperand(inst.Src0, lane))
		set := uint32(state.ReadOperand(inst.Src1, lane))
		clear := uint32(state.ReadOperand(inst.Src2, lane))
		state.WriteOperand(inst.Dst, lane, uint64((mask&set)|(clear&^mask)))
	}
}

func (u *ALU) runVPKMOVB32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	loShift := uint(inst.OpSel&1) * 32
	hiShift := uint((inst.OpSel>>1)&1) * 32
	for lane := 0; lane < 64; lane++ {
		if exec&(1<<uint(lane)) == 0 {
			continue
		}
		lo := uint32(state.ReadOperand(inst.Src0, lane) >> loShift)
		hi := uint32(state.ReadOperand(inst.Src1, lane) >> hiShift)
		state.WriteOperand(inst.Dst, lane, uint64(lo)|uint64(hi)<<32)
	}
}

func (u *ALU) runVXADU32(state emu.InstEmuState) {
	inst := state.Inst()
	for lane := 0; lane < 64; lane++ {
		if state.EXEC()&(1<<uint(lane)) != 0 {
			a := uint32(state.ReadOperand(inst.Src0, lane))
			b := uint32(state.ReadOperand(inst.Src1, lane))
			c := uint32(state.ReadOperand(inst.Src2, lane))
			state.WriteOperand(inst.Dst, lane, uint64((a^b)+c))
		}
	}
}

func (u *ALU) runLDEXPF64(state emu.InstEmuState) {
	inst := state.Inst()
	for lane := 0; lane < 64; lane++ {
		if state.EXEC()&(1<<uint(lane)) != 0 {
			value := math.Float64frombits(readModifiedF64(state, inst.Src0, lane, 0))
			power := int(int32(state.ReadOperand(inst.Src1, lane)))
			state.WriteOperand(inst.Dst, lane, math.Float64bits(math.Ldexp(value, power)))
		}
	}
}

func (u *ALU) runVANDORB32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for lane := 0; lane < 64; lane++ {
		if exec&(1<<uint(lane)) == 0 {
			continue
		}
		a := uint32(state.ReadOperand(inst.Src0, lane))
		b := uint32(state.ReadOperand(inst.Src1, lane))
		c := uint32(state.ReadOperand(inst.Src2, lane))
		state.WriteOperand(inst.Dst, lane, uint64((a&b)|c))
	}
}

func (u *ALU) runVOR3B32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		src2 := uint32(state.ReadOperand(inst.Src2, i))
		result := src0 | src1 | src2
		state.WriteOperand(inst.Dst, i, uint64(result))
	}
}

// runVLDEXPF32 implements v_ldexp_f32.
// D.f = S0.f * 2^(S1.i32)
func (u *ALU) runVLDEXPF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		exp := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, i)))
		result := float32(math.Ldexp(float64(src0), int(exp)))
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(result)))
	}
}

func (u *ALU) runPackB32F16(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for lane := 0; lane < 64; lane++ {
		if exec&(1<<uint(lane)) == 0 {
			continue
		}
		lo := uint16(state.ReadOperand(inst.Src0, lane))
		hi := uint16(state.ReadOperand(inst.Src1, lane))
		state.WriteOperand(inst.Dst, lane, uint64(lo)|uint64(hi)<<16)
	}
}

func packedHalfInput(value uint32, selectHigh bool, negate bool) float32 {
	if selectHigh {
		value >>= 16
	}
	if negate {
		value ^= 0x8000
	}
	return float16ToFloat32(uint16(value))
}

func packedF16Result(opcode insts.Opcode, sources [3]uint32, selectors, neg int, clamp bool) uint16 {
	var input [3]float32
	for i, source := range sources {
		mask := 1 << i
		input[i] = packedHalfInput(source, selectors&mask != 0, neg&mask != 0)
	}
	var result uint16
	if opcode == 910 {
		result = bitops.FMAF16(input[0], input[1], input[2])
	} else {
		result = bitops.Float64ToFloat16(float64(input[0]) + float64(input[1]))
	}
	return clampF16(result, clamp)
}

func clampF16(value uint16, clamp bool) uint16 {
	if !clamp {
		return value
	}
	if value&0x8000 != 0 || value&0x7fff > 0x7c00 {
		return 0
	}
	if value > 0x3c00 {
		return 0x3c00
	}
	return value
}

func (u *ALU) runPackedF16(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for lane := 0; lane < 64; lane++ {
		if exec&(1<<uint(lane)) == 0 {
			continue
		}
		sources := [3]uint32{
			uint32(state.ReadOperand(inst.Src0, lane)),
			uint32(state.ReadOperand(inst.Src1, lane)),
		}
		if inst.Src2 != nil {
			sources[2] = uint32(state.ReadOperand(inst.Src2, lane))
		}
		lo := packedF16Result(inst.Opcode, sources, inst.OpSel, inst.Neg, inst.Clamp)
		// The high-half negation bits overlap the VOP3A absolute-value field.
		hi := packedF16Result(inst.Opcode, sources, inst.OpSelHi, inst.Abs, inst.Clamp)
		state.WriteOperand(inst.Dst, lane, uint64(lo)|uint64(hi)<<16)
	}
}

func mixedF16Input(value uint32, source int, inst *insts.Inst) float32 {
	mask := 1 << source
	converted := math.Float32frombits(value)
	if inst.OpSelHi&mask != 0 {
		converted = packedHalfInput(value, inst.OpSel&mask != 0, false)
	}
	// For mixed inputs the same encoding controls absolute value.
	if inst.Abs&mask != 0 {
		converted = float32(math.Abs(float64(converted)))
	}
	if inst.Neg&mask != 0 {
		converted = -converted
	}
	return converted
}

func (u *ALU) runMixedF16(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	shift := uint(0)
	if inst.Opcode == 930 {
		shift = 16
	}
	for lane := 0; lane < 64; lane++ {
		if exec&(1<<uint(lane)) == 0 {
			continue
		}
		a := mixedF16Input(uint32(state.ReadOperand(inst.Src0, lane)), 0, inst)
		b := mixedF16Input(uint32(state.ReadOperand(inst.Src1, lane)), 1, inst)
		c := mixedF16Input(uint32(state.ReadOperand(inst.Src2, lane)), 2, inst)
		converted := float32ToFloat16(bitops.FMAF32(a, b, c))
		result := clampF16(converted, inst.Clamp)
		dst := uint32(state.ReadOperand(inst.Dst, lane)) &^ (uint32(0xffff) << shift)
		state.WriteOperand(inst.Dst, lane, uint64(dst|uint32(result)<<shift))
	}
}

func (u *ALU) runVMULF32VOP3a(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		dst := src0 * src1
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runVMADF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		src2 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src2, i), 2, inst)))
		res := src0*src1 + src2
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(res)))
	}
}

func (u *ALU) runVMADI32I24(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := int32(bitops.SignExt(
			bitops.ExtractBitsFromU64(state.ReadOperand(inst.Src0, i), 0, 23), 23))
		src1 := int32(bitops.SignExt(
			bitops.ExtractBitsFromU64(state.ReadOperand(inst.Src1, i), 0, 23), 23))
		src2 := int32(state.ReadOperand(inst.Src2, i))
		state.WriteOperand(inst.Dst, i, uint64(src0*src1+src2))
	}
}

func (u *ALU) runVMADU64U32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		s0 := uint64(uint32(state.ReadOperand(inst.Src0, i)))
		s1 := uint64(uint32(state.ReadOperand(inst.Src1, i)))
		s2 := state.ReadOperand(inst.Src2, i)
		state.WriteOperand(inst.Dst, i, s0*s1+s2)
	}
}

func (u *ALU) runVMULLOU32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		state.WriteOperand(inst.Dst, i, src0*src1)
	}
}

func (u *ALU) runVMULHIU32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		state.WriteOperand(inst.Dst, i, (src0*src1)>>32)
	}
}

func (u *ALU) runVLSHLREVB64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		shift := state.ReadOperand(inst.Src0, i)
		src := state.ReadOperand(inst.Src1, i)
		state.WriteOperand(inst.Dst, i, src<<shift)
	}
}

func (u *ALU) runVASHRREVI64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		shift := state.ReadOperand(inst.Src0, i)
		src := state.ReadOperand(inst.Src1, i)
		state.WriteOperand(inst.Dst, i, emu.Int64ToBits(emu.AsInt64(src)>>shift))
	}
}

func (u *ALU) runVADDF64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float64frombits(readModifiedF64(state, inst.Src0, i, 0))
		src1 := math.Float64frombits(readModifiedF64(state, inst.Src1, i, 1))
		dst := src0 + src1
		state.WriteOperand(inst.Dst, i, math.Float64bits(dst))
	}
}

func (u *ALU) runVFMAF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		src2 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src2, i), 2, inst)))
		dst := bitops.FMAF32(src0, src1, src2)
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runVFMAF64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float64frombits(readModifiedF64(state, inst.Src0, i, 0))
		src1 := math.Float64frombits(readModifiedF64(state, inst.Src1, i, 1))
		src2 := math.Float64frombits(readModifiedF64(state, inst.Src2, i, 2))
		dst := math.FMA(src0, src1, src2)
		state.WriteOperand(inst.Dst, i, math.Float64bits(dst))
	}
}

func (u *ALU) runVMIN3F32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		src2 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src2, i), 2, inst)))
		dst := src0
		if src1 < dst {
			dst = src1
		}
		if src2 < dst {
			dst = src2
		}
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runVMIN3I32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, i)))
		src2 := emu.AsInt32(uint32(state.ReadOperand(inst.Src2, i)))
		dst := src0
		if src1 < dst {
			dst = src1
		}
		if src2 < dst {
			dst = src2
		}
		state.WriteOperand(inst.Dst, i, uint64(emu.Int32ToBits(dst)))
	}
}

func (u *ALU) runVMIN3U32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		src2 := uint32(state.ReadOperand(inst.Src2, i))
		dst := src0
		if src1 < dst {
			dst = src1
		}
		if src2 < dst {
			dst = src2
		}
		state.WriteOperand(inst.Dst, i, uint64(dst))
	}
}

func (u *ALU) runVMAX3F32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		src2 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src2, i), 2, inst)))
		dst := src0
		if src1 > dst {
			dst = src1
		}
		if src2 > dst {
			dst = src2
		}
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runVMAX3I32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, i)))
		src2 := emu.AsInt32(uint32(state.ReadOperand(inst.Src2, i)))
		dst := src0
		if src1 > dst {
			dst = src1
		}
		if src2 > dst {
			dst = src2
		}
		state.WriteOperand(inst.Dst, i, uint64(emu.Int32ToBits(dst)))
	}
}

func (u *ALU) runVMAX3U32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		src2 := uint32(state.ReadOperand(inst.Src2, i))
		dst := src0
		if src1 > dst {
			dst = src1
		}
		if src2 > dst {
			dst = src2
		}
		state.WriteOperand(inst.Dst, i, uint64(dst))
	}
}

func (u *ALU) runVMED3F32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		src2 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src2, i), 2, inst)))
		list := []float64{float64(src0), float64(src1), float64(src2)}
		sort.Float64s(list)
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(float32(list[1]))))
	}
}

func (u *ALU) runVMED3I32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, i)))
		src2 := emu.AsInt32(uint32(state.ReadOperand(inst.Src2, i)))
		list := []int{int(src0), int(src1), int(src2)}
		sort.Ints(list)
		dst := int32(list[1])
		state.WriteOperand(inst.Dst, i, uint64(emu.Int32ToBits(dst)))
	}
}

func (u *ALU) runVMED3U32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		src2 := uint32(state.ReadOperand(inst.Src2, i))
		dst := median3Uint32(src0, src1, src2)
		state.WriteOperand(inst.Dst, i, uint64(dst))
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
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float64frombits(readModifiedF64(state, inst.Src0, i, 0))
		src1 := math.Float64frombits(readModifiedF64(state, inst.Src1, i, 1))
		dst := src0 * src1
		state.WriteOperand(inst.Dst, i, math.Float64bits(dst))
	}
}

func (u *ALU) runVDIVFMASF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	vcc := state.VCC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		vccBit := (vcc >> uint(i)) & 1
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		src2 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src2, i), 2, inst)))
		// v_div_fmas_f32: Part of software division - final step
		// Simplified: if VCC[i], scale by 2^32, else normal FMA
		var dst float32
		if vccBit == 1 {
			dst = float32(math.Pow(2.0, 32)) * (src0*src1 + src2)
		} else {
			dst = src0*src1 + src2
		}
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runVDIVFMASF64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	vcc := state.VCC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		vccBit := (vcc >> uint(i)) & 1
		src0 := math.Float64frombits(readModifiedF64(state, inst.Src0, i, 0))
		src1 := math.Float64frombits(readModifiedF64(state, inst.Src1, i, 1))
		src2 := math.Float64frombits(readModifiedF64(state, inst.Src2, i, 2))
		var dst float64
		if vccBit == 1 {
			dst = math.Pow(2.0, 64) * (src0*src1 + src2)
		} else {
			dst = src0*src1 + src2
		}
		state.WriteOperand(inst.Dst, i, math.Float64bits(dst))
	}
}

func (u *ALU) runVDIVFIXUPF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src0, i), 0, inst)))
		src1 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src1, i), 1, inst)))
		src2 := math.Float32frombits(uint32(applyF32Modifier(state.ReadOperand(inst.Src2, i), 2, inst)))
		// SRC1 is the denominator and SRC2 is the numerator. The
		// approximation may be undefined when either input is exceptional.
		dst := src0
		if src1 == 0 || src2 == 0 ||
			math.IsNaN(float64(src1)) || math.IsNaN(float64(src2)) ||
			math.IsInf(float64(src1), 0) || math.IsInf(float64(src2), 0) {
			dst = src2 / src1
		}
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runVDIVFIXUPF64(state emu.InstEmuState) {
	inst := state.Inst()
	for lane := 0; lane < 64; lane++ {
		if state.EXEC()&(1<<uint(lane)) == 0 {
			continue
		}
		estimate := readModifiedF64(state, inst.Src0, lane, 0)
		denominator := readModifiedF64(state, inst.Src1, lane, 1)
		numerator := readModifiedF64(state, inst.Src2, lane, 2)
		state.WriteOperand(inst.Dst, lane, u.calculateDivFixUpF64(estimate, denominator, numerator))
	}
}

func (u *ALU) calculateDivFixUpF64(estimateBits, denominatorBits, numeratorBits uint64) uint64 {
	denominator := math.Float64frombits(denominatorBits)
	numerator := math.Float64frombits(numeratorBits)
	if denominator == 0 || numerator == 0 || math.IsNaN(denominator) || math.IsNaN(numerator) ||
		math.IsInf(denominator, 0) || math.IsInf(numerator, 0) {
		return math.Float64bits(numerator / denominator)
	}
	const sign = uint64(1) << 63
	return estimateBits&^sign | (denominatorBits^numeratorBits)&sign
}

// Packed FP32 operations broadcast non-register constants to both words.
func readPackedF32Operand(state emu.InstEmuState, operand *insts.Operand, lane int) uint64 {
	bits := state.ReadOperand(operand, lane)
	if operand.OperandType == insts.RegOperand {
		return bits
	}
	word := uint64(uint32(bits))
	return word | word<<32
}

// Packed FP32 encodes the word selectors and signs independently for each
// result. The low instruction word's normal ABS bits encode NEG_HI here.
func readPackedF32Half(
	state emu.InstEmuState,
	operand *insts.Operand,
	lane, source, selectBits, negBits int,
) float32 {
	bits := readPackedF32Operand(state, operand, lane)
	if selectBits&(1<<source) != 0 {
		bits >>= 32
	}
	if negBits&(1<<source) != 0 {
		bits ^= 1 << 31
	}
	return math.Float32frombits(uint32(bits))
}

func readPackedF32Pair(
	state emu.InstEmuState,
	operand *insts.Operand,
	lane, source int,
) (lo, hi float32) {
	inst := state.Inst()
	return readPackedF32Half(state, operand, lane, source, inst.OpSel, inst.Neg),
		readPackedF32Half(state, operand, lane, source, inst.OpSelHi, inst.Abs)
}

func writePackedF32Pair(state emu.InstEmuState, lane int, lo, hi float32) {
	bits := uint64(math.Float32bits(lo)) | uint64(math.Float32bits(hi))<<32
	state.WriteOperand(state.Inst().Dst, lane, bits)
}

func (u *ALU) runVPKFMAF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		aLo, aHi := readPackedF32Pair(state, inst.Src0, i, 0)
		bLo, bHi := readPackedF32Pair(state, inst.Src1, i, 1)
		cLo, cHi := readPackedF32Pair(state, inst.Src2, i, 2)
		writePackedF32Pair(state, i, bitops.FMAF32(aLo, bLo, cLo), bitops.FMAF32(aHi, bHi, cHi))
	}
}

func (u *ALU) runVPKMULF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		aLo, aHi := readPackedF32Pair(state, inst.Src0, i, 0)
		bLo, bHi := readPackedF32Pair(state, inst.Src1, i, 1)
		writePackedF32Pair(state, i, aLo*bLo, aHi*bHi)
	}
}

func (u *ALU) runVPKADDF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		aLo, aHi := readPackedF32Pair(state, inst.Src0, i, 0)
		bLo, bHi := readPackedF32Pair(state, inst.Src1, i, 1)
		writePackedF32Pair(state, i, aLo+bLo, aHi+bHi)
	}
}
