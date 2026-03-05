package emu

import (
	"log"
	"math"
	"sort"

	"github.com/sarchlab/mgpusim/v4/amd/bitops"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// applyF32Modifier applies abs and neg modifiers for a float32 source operand.
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

// applyF64Modifier applies abs and neg modifiers for a float64 source operand.
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

// applyB32Modifier applies neg modifier for a B32 (integer) source operand.
func applyB32Modifier(val uint64, srcIdx int, inst *insts.Inst) uint64 {
	if inst.Neg&(1<<uint(srcIdx)) != 0 {
		v := asInt32(uint32(val))
		v = -v
		return uint64(int32ToBits(v))
	}
	return val
}

//nolint:gocyclo,funlen
func (u *ALUImpl) runVOP3A(state InstEmuState) {
	inst := state.Inst()

	u.vop3aPreprocess(state)

	switch inst.Opcode {
	case 65: // 0x41
		u.runVCmpLtF32VOP3a(state)
	case 68: //0x44
		u.runVCmpGtF32VOP3a(state)
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
	case 456:
		u.runVBFEU32(state)
	case 457:
		u.runVBFEI32(state)
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
	case 479:
		u.runVDIVFIXUPF64(state)
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
	case 655:
		u.runVLSHLREVB64(state)
	case 657:
		u.runVASHRREVI64(state)
	case 511:
		u.runVADD3U32(state)
	case 520:
		u.runVLSHLADDU64(state)
	default:
		log.Panicf("Opcode %d for VOP3a format is not implemented", inst.Opcode)
	}
	u.vop3aPostprocess(state)
}

func (u *ALUImpl) vop3aPreprocess(state InstEmuState) {
	// No-op: modifiers are now applied inline via applyF32Modifier/applyF64Modifier/applyB32Modifier
}

func (u *ALUImpl) vop3aPostprocess(state InstEmuState) {
	inst := state.Inst()

	if inst.Omod != 0 {
		log.Panic("Output modifiers are not supported.")
	}
}

func (u *ALUImpl) runVCmpLtF32VOP3a(state InstEmuState) {
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

func (u *ALUImpl) runVCmpGtF32VOP3a(state InstEmuState) {
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

func (u *ALUImpl) runVCmpNltF32VOP3a(state InstEmuState) {
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

func (u *ALUImpl) runVCmpLtI32VOP3a(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := asInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := asInt32(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 < src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALUImpl) runVCmpLeI32VOP3a(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := asInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := asInt32(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 <= src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALUImpl) runVCmpGtI32VOP3a(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := asInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := asInt32(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 > src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALUImpl) runVCmpGEI32VOP3a(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := asInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := asInt32(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 >= src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALUImpl) runVCmpLtU32VOP3a(state InstEmuState) {
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

func (u *ALUImpl) runVCmpEqU32VOP3a(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if uint32(src0) == uint32(src1) {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALUImpl) runVCmpLeU32VOP3a(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 <= src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALUImpl) runVCmpGtU32VOP3a(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 > src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALUImpl) runVCmpLgU32VOP3a(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 != src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALUImpl) runVCmpGeU32VOP3a(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var dst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 >= src1 {
			dst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALUImpl) runVCmpLtU64VOP3a(state InstEmuState) {
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

func (u *ALUImpl) runVCNDMASKB32VOP3a(state InstEmuState) {
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

func (u *ALUImpl) runVSUBF32VOP3a(state InstEmuState) {
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

func (u *ALUImpl) runVMADF32(state InstEmuState) {
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

func (u *ALUImpl) runVMADI32I24(state InstEmuState) {
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

func (u *ALUImpl) runVMADU64U32(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		src2 := state.ReadOperand(inst.Src2, i)
		state.WriteOperand(inst.Dst, i, src0*src1+src2)
	}
}

func (u *ALUImpl) runVBFEU32(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		offset := uint32(state.ReadOperand(inst.Src1, i)) & 0x1F
		width := uint32(state.ReadOperand(inst.Src2, i)) & 0x1F
		if width == 0 {
			state.WriteOperand(inst.Dst, i, 0)
		} else if offset+width < 32 {
			mask := uint32((1 << width) - 1)
			state.WriteOperand(inst.Dst, i, uint64((src0>>offset)&mask))
		} else {
			state.WriteOperand(inst.Dst, i, uint64(src0>>offset))
		}
	}
}

func (u *ALUImpl) runVBFEI32(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		offset := uint32(state.ReadOperand(inst.Src1, i)) & 0x1F
		width := uint32(state.ReadOperand(inst.Src2, i)) & 0x1F
		if width == 0 {
			state.WriteOperand(inst.Dst, i, 0)
		} else if offset+width < 32 {
			mask := uint32((1 << width) - 1)
			extracted := (src0 >> offset) & mask
			if extracted&(1<<(width-1)) != 0 {
				signExtMask := uint32(0xFFFFFFFF << width)
				state.WriteOperand(inst.Dst, i, uint64(int32(extracted|signExtMask)))
			} else {
				state.WriteOperand(inst.Dst, i, uint64(extracted))
			}
		} else {
			extracted := src0 >> offset
			state.WriteOperand(inst.Dst, i, uint64(int32(extracted)))
		}
	}
}

func (u *ALUImpl) runVADD3U32(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		src2 := uint32(state.ReadOperand(inst.Src2, i))
		state.WriteOperand(inst.Dst, i, uint64(src0+src1+src2))
	}
}

func (u *ALUImpl) runVLSHLADDU64(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := uint32(state.ReadOperand(inst.Src1, i)) & 0x3F
		src2 := state.ReadOperand(inst.Src2, i)
		state.WriteOperand(inst.Dst, i, (src0<<src1)+src2)
	}
}

func (u *ALUImpl) runVMULLOU32(state InstEmuState) {
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

func (u *ALUImpl) runVMULHIU32(state InstEmuState) {
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

func (u *ALUImpl) runVLSHLREVB64(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		state.WriteOperand(inst.Dst, i, src1<<src0)
	}
}

func (u *ALUImpl) runVASHRREVI64(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		state.WriteOperand(inst.Dst, i, int64ToBits(asInt64(src1)>>src0))
	}
}

func (u *ALUImpl) runVADDF64(state InstEmuState) {
	inst := state.Inst()
	if inst.IsSdwa {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
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

func (u *ALUImpl) runVFMAF64(state InstEmuState) {
	inst := state.Inst()
	if inst.IsSdwa {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
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

func (u *ALUImpl) runVMIN3F32(state InstEmuState) {
	inst := state.Inst()
	if inst.IsSdwa {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
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

func (u *ALUImpl) runVMIN3I32(state InstEmuState) {
	inst := state.Inst()
	if inst.IsSdwa {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := asInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := asInt32(uint32(state.ReadOperand(inst.Src1, i)))
		src2 := asInt32(uint32(state.ReadOperand(inst.Src2, i)))
		dst := src0
		if src1 < dst {
			dst = src1
		}
		if src2 < dst {
			dst = src2
		}
		state.WriteOperand(inst.Dst, i, uint64(int32ToBits(dst)))
	}
}

func (u *ALUImpl) runVMIN3U32(state InstEmuState) {
	inst := state.Inst()
	if inst.IsSdwa {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
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

func (u *ALUImpl) runVMAX3F32(state InstEmuState) {
	inst := state.Inst()
	if inst.IsSdwa {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
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

func (u *ALUImpl) runVMAX3I32(state InstEmuState) {
	inst := state.Inst()
	if inst.IsSdwa {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := asInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := asInt32(uint32(state.ReadOperand(inst.Src1, i)))
		src2 := asInt32(uint32(state.ReadOperand(inst.Src2, i)))
		dst := src0
		if src1 > dst {
			dst = src1
		}
		if src2 > dst {
			dst = src2
		}
		state.WriteOperand(inst.Dst, i, uint64(int32ToBits(dst)))
	}
}

func (u *ALUImpl) runVMAX3U32(state InstEmuState) {
	inst := state.Inst()
	if inst.IsSdwa {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
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

func (u *ALUImpl) runVMED3F32(state InstEmuState) {
	inst := state.Inst()
	if inst.IsSdwa {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
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

func (u *ALUImpl) runVMED3I32(state InstEmuState) {
	inst := state.Inst()
	if inst.IsSdwa {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := asInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := asInt32(uint32(state.ReadOperand(inst.Src1, i)))
		src2 := asInt32(uint32(state.ReadOperand(inst.Src2, i)))
		list := []int{int(src0), int(src1), int(src2)}
		sort.Ints(list)
		dst := int32(list[1])
		state.WriteOperand(inst.Dst, i, uint64(int32ToBits(dst)))
	}
}

func (u *ALUImpl) runVMED3U32(state InstEmuState) {
	inst := state.Inst()
	if inst.IsSdwa {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
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

func (u *ALUImpl) runVMULF64(state InstEmuState) {
	inst := state.Inst()
	if inst.IsSdwa {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
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

func (u *ALUImpl) runVDIVFMASF64(state InstEmuState) {
	inst := state.Inst()
	if inst.IsSdwa {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		vccVal := state.VCC() & (1 << uint(i))
		src0 := math.Float64frombits(applyF64Modifier(state.ReadOperand(inst.Src0, i), 0, inst))
		src1 := math.Float64frombits(applyF64Modifier(state.ReadOperand(inst.Src1, i), 1, inst))
		src2 := math.Float64frombits(applyF64Modifier(state.ReadOperand(inst.Src2, i), 2, inst))
		var dst float64
		if vccVal == 1 {
			dst = math.Pow(2.0, 64) * (src0*src1 + src2)
		} else {
			dst = src0*src1 + src2
		}
		state.WriteOperand(inst.Dst, i, math.Float64bits(dst))
	}
}

func (u *ALUImpl) runVDIVFIXUPF64(state InstEmuState) {
	inst := state.Inst()
	if inst.IsSdwa {
		log.Panicf("SDWA for VOP3A instruction opcode %d not implemented \n", inst.Opcode)
	}
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
func (u *ALUImpl) calculateDivFixUpF64(
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

func (u *ALUImpl) isInfByInf(src1, src2 float64) bool {
	return (math.Abs(src1) == 0x7FF0000000000000 ||
		math.Abs(src1) == 0xFFF0000000000000) &&
		(math.Abs(src2) == 0x7FF0000000000000 ||
			math.Abs(src2) == 0xFFF0000000000000)
}

func (u *ALUImpl) isDIVFIXUPF64Overflow(
	exponentSrc1, exponentSrc2 uint64,
) bool {
	return int64(exponentSrc2-exponentSrc1) < -1075 ||
		exponentSrc1 == 2047
}
