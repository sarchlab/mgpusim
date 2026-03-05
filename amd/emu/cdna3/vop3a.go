package cdna3

import (
	"log"
	"math"
	"sort"
	"strings"

	"github.com/sarchlab/mgpusim/v4/amd/bitops"
	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
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
	case 65: // 0x41
		u.runVCmpLtF32VOP3a(state)
	case 68: // 0x44
		u.runVCmpGtF32VOP3a(state)
	case 70: // 0x46
		u.runVCmpLeF32VOP3a(state)
	case 16: // v_cmp_class_f32
		u.runVCmpClassF32VOP3a(state)
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
	case 261:
		u.runVMULF32VOP3a(state)
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
	case 510:
		u.runVADDLSHLU32(state)
	case 511:
		u.runVADD3U32(state)
	case 512:
		u.runVLSHLORB32(state)
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

	if strings.HasPrefix(inst.InstName, "v_pk_") {
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
		isNaN && frac&(1<<22) == 0,  // 0: signaling NaN
		isNaN && frac&(1<<22) != 0,  // 1: quiet NaN
		isInf && sign,               // 2: negative infinity
		isNorm && sign,              // 3: negative normal
		isDenorm && sign,            // 4: negative denormal
		isZero && sign,              // 5: negative zero
		isZero && !sign,             // 6: positive zero
		isDenorm && !sign,           // 7: positive denormal
		isNorm && !sign,             // 8: positive normal
		isInf && !sign,              // 9: positive infinity
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
		src0 := math.Float64frombits(applyF64Modifier(state.ReadOperand(inst.Src0, i), 0, inst))
		src1 := math.Float64frombits(applyF64Modifier(state.ReadOperand(inst.Src1, i), 1, inst))
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
		dst := src0*src1 + src2
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
		src0 := math.Float64frombits(applyF64Modifier(state.ReadOperand(inst.Src0, i), 0, inst))
		src1 := math.Float64frombits(applyF64Modifier(state.ReadOperand(inst.Src1, i), 1, inst))
		src2 := math.Float64frombits(applyF64Modifier(state.ReadOperand(inst.Src2, i), 2, inst))
		dst := src0*src1 + src2
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
		src0 := math.Float64frombits(applyF64Modifier(state.ReadOperand(inst.Src0, i), 0, inst))
		src1 := math.Float64frombits(applyF64Modifier(state.ReadOperand(inst.Src1, i), 1, inst))
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
		src0 := math.Float64frombits(applyF64Modifier(state.ReadOperand(inst.Src0, i), 0, inst))
		src1 := math.Float64frombits(applyF64Modifier(state.ReadOperand(inst.Src1, i), 1, inst))
		src2 := math.Float64frombits(applyF64Modifier(state.ReadOperand(inst.Src2, i), 2, inst))
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
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runVDIVFIXUPF64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := applyF64Modifier(state.ReadOperand(inst.Src0, i), 0, inst)
		src1 := applyF64Modifier(state.ReadOperand(inst.Src1, i), 1, inst)
		src2 := applyF64Modifier(state.ReadOperand(inst.Src2, i), 2, inst)
		state.WriteOperand(inst.Dst, i, u.calculateDivFixUpF64(src0, src1, src2))
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
	inst := state.Inst()
	exec := state.EXEC()

	// VOP3P encoding: OpSel (bits 11-14) and OpSelHi (bits 59-60)
	op_sel := inst.OpSel
	op_sel_hi := inst.OpSelHi

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		src0Bits := state.ReadOperand(inst.Src0, i)
		src1Bits := state.ReadOperand(inst.Src1, i)
		src2Bits := state.ReadOperand(inst.Src2, i)

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
		state.WriteOperand(inst.Dst, i, dstBits)
	}
}

func (u *ALU) runVPKMULF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()

	// VOP3P encoding for 2-source packed f32 operations:
	// OpSel (bits 11-14, 4 bits total) controls ALL source selection:
	// OpSel[0] (bit 0): src0 word select for lower result (0=lo, 1=hi)
	// OpSel[1] (bit 1): src1 word select for lower result (0=lo, 1=hi)
	// OpSel[2] (bit 2): src0 word select for upper result (0=lo, 1=hi)
	// OpSel[3] (bit 3): src1 word select for upper result (0=lo, 1=hi)
	// Note: OpSelHi is NOT used for 2-source packed ops, only for 3-source FMA
	op_sel := inst.OpSel

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		src0Bits := state.ReadOperand(inst.Src0, i)
		src1Bits := state.ReadOperand(inst.Src1, i)

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
		state.WriteOperand(inst.Dst, i, dstBits)
	}
}

func (u *ALU) runVPKADDF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()

	// VOP3P encoding for 2-source packed f32 operations:
	// OpSel (bits 11-14, 4 bits total) controls ALL source selection:
	// OpSel[0] (bit 0): src0 word select for lower result (0=lo, 1=hi)
	// OpSel[1] (bit 1): src1 word select for lower result (0=lo, 1=hi)
	// OpSel[2] (bit 2): src0 word select for upper result (0=lo, 1=hi)
	// OpSel[3] (bit 3): src1 word select for upper result (0=lo, 1=hi)
	// Note: OpSelHi is NOT used for 2-source packed ops, only for 3-source FMA
	op_sel := inst.OpSel

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		src0Bits := state.ReadOperand(inst.Src0, i)
		src1Bits := state.ReadOperand(inst.Src1, i)

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
		state.WriteOperand(inst.Dst, i, dstBits)
	}
}
