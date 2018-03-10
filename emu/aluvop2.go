package emu

import (
	"log"
	"math"
)

func (u *ALUImpl) runVOP2(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runVCNDMASKB32(state)
	case 1:
		u.runVADDF32(state)
	case 2:
		u.runVSUBF32(state)
	case 3:
		u.runVSUBREVF32(state)
	case 4:
		u.runVMULF32(state)
	case 5:
		u.runVMULF32(state)
	case 16:
		u.runVLSHRREVB32(state)
	case 17:
		u.runVASHRREVI32(state)
	case 18:
		u.runVLSHLREVB32(state)
	case 19:
		u.runVANDB32(state)
	case 20:
		u.runVORB32(state)
	case 21:
		u.runVXORB32(state)
	case 22:
		u.runVMACF32(state)
	case 25:
		u.runVADDI32(state)
	case 26:
		u.runVSUBI32(state)
	case 27:
		u.runVSUBREVI32(state)
	case 28:
		u.runVADDCU32(state)
	default:
		log.Panicf("Opcode %d for VOP2 format is not implemented", inst.Opcode)
	}
}

func (u *ALUImpl) runVCNDMASKB32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	inst := state.Inst()
	if inst.IsSdwa == false {
		var i uint
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}

			if (sp.VCC & (1 << i)) > 0 {
				sp.DST[i] = sp.SRC1[i]
			} else {
				sp.DST[i] = sp.SRC0[i]
			}
		}
	} else {
		log.Panicf("SDWA for VOP2 instruction opcode %d not implemented \n", inst.Opcode)
	}

}

func (u *ALUImpl) runVADDF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	inst := state.Inst()
	if inst.IsSdwa == false {
		var i uint
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}

			src0 := math.Float32frombits(uint32(sp.SRC0[i]))
			src1 := math.Float32frombits(uint32(sp.SRC1[i]))
			dst := src0 + src1
			sp.DST[i] = uint64(math.Float32bits(dst))
		}
	} else {
		log.Panicf("SDWA for VOP2 instruction opcode  %d not implemented \n", inst.Opcode)

	}
}

func (u *ALUImpl) runVSUBF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	inst := state.Inst()
	if inst.IsSdwa == false {
		var i uint
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}

			src0 := math.Float32frombits(uint32(sp.SRC0[i]))
			src1 := math.Float32frombits(uint32(sp.SRC1[i]))
			dst := src0 - src1
			sp.DST[i] = uint64(math.Float32bits(dst))
		}
	} else {
		log.Panicf("SDWA for VOP2 instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func (u *ALUImpl) runVSUBREVF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	inst := state.Inst()
	if inst.IsSdwa == false {
		var i uint
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}

			src0 := math.Float32frombits(uint32(sp.SRC0[i]))
			src1 := math.Float32frombits(uint32(sp.SRC1[i]))
			dst := src1 - src0
			sp.DST[i] = uint64(math.Float32bits(dst))
		}
	} else {
		log.Panicf("SDWA for VOP2 instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func (u *ALUImpl) runVMULF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	inst := state.Inst()
	if inst.IsSdwa == false {
		var i uint
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}

			src0 := math.Float32frombits(uint32(sp.SRC0[i]))
			src1 := math.Float32frombits(uint32(sp.SRC1[i]))
			dst := src0 * src1
			sp.DST[i] = uint64(math.Float32bits(dst))
		}
	} else {
		log.Panicf("SDWA for VOP2 instruction opcode %d not implemented \n", inst.Opcode)

	}
}

func (u *ALUImpl) runVLSHRREVB32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	inst := state.Inst()
	if inst.IsSdwa == false {
		var i uint
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}
			src0 := sp.SRC0[i]
			src1 := sp.SRC1[i]
			dst := src1 >> (src0 & 0x1f)
			sp.DST[i] = uint64(dst)
		}
	} else {
		log.Panicf("SDWA for VOP2 instruction opcode %d not implemented \n", inst.Opcode)

	}
}

func (u *ALUImpl) runVASHRREVI32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	inst := state.Inst()
	if inst.IsSdwa == false {
		var i uint
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}
			src0 := uint32(sp.SRC0[i])
			src1 := int32(sp.SRC1[i])
			dst := src1 >> (src0 & 0X1f)
			sp.DST[i] = uint64(dst)
		}
	} else {
		log.Panicf("SDWA for VOP2 instruction opcode  %d not implemented \n", inst.Opcode)

	}
}

func (u *ALUImpl) runVLSHLREVB32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	inst := state.Inst()
	if inst.IsSdwa == false {
		var i uint
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}
			src0 := uint32(sp.SRC0[i])
			src1 := uint32(sp.SRC1[i])
			dst := src1 << (src0 & 0x1f)
			sp.DST[i] = uint64(dst)
		}
	} else {
		log.Panicf("SDWA for VOP2 instruction opcode  %d not implemented \n", inst.Opcode)

	}
}

func (u *ALUImpl) runVANDB32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	inst := state.Inst()
	var i uint
	if inst.IsSdwa == false {
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}
			src0 := uint32(sp.SRC0[i])
			src1 := uint32(sp.SRC1[i])
			dst := src0 & src1
			sp.DST[i] = uint64(dst)
		}
	} else {
		for i = 0; i < 64; i++ {
			src0 := uint32(sp.SRC0[i]) & inst.Src0_Sel
			src1 := uint32(sp.SRC1[i]) & inst.Src1_Sel
			dst  := (src0 & src1) & inst.Dst_Sel
			sp.DST[i] = uint64(dst)
		}
	}
}

func (u *ALUImpl) runVORB32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	inst := state.Inst()
	var i uint
	if inst.IsSdwa == false {
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}
			src0 := uint32(sp.SRC0[i])
			src1 := uint32(sp.SRC1[i])
			dst := src0 | src1
			sp.DST[i] = uint64(dst)
		}
	} else {
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}
			src0 := uint32(sp.SRC0[i]) & inst.Src0_Sel
			src1 := uint32(sp.SRC1[i]) & inst.Src1_Sel
			dst := (src0 & src1) | inst.Dst_Sel
			sp.DST[i] = uint64(dst)
		}
	}
}

func (u *ALUImpl) runVXORB32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	inst := state.Inst()
	var i uint
	if inst.IsSdwa == false {
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}
			src0 := uint32(sp.SRC0[i])
			src1 := uint32(sp.SRC1[i])
			dst := src0 ^ src1
			sp.DST[i] = uint64(dst)
		}
	} else {
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}
			src0 := uint32(sp.SRC0[i]) & inst.Src0_Sel
			src1 := uint32(sp.SRC1[i]) & inst.Src1_Sel
			dst := (src0 ^ src1) & inst.Dst_Sel
			sp.DST[i] = uint64(dst)
		}
	}
}

func (u *ALUImpl) runVMACF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	inst := state.Inst()
	var dst float32
	var src0 float32
	var src1 float32

	var i uint
	if inst.IsSdwa == false {
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}

			dst = asFloat32(uint32(sp.DST[i]))
			src0 = asFloat32(uint32(sp.SRC0[i]))
			src1 = asFloat32(uint32(sp.SRC1[i]))
			dst += src0 * src1
			sp.DST[i] = uint64(float32ToBits(dst))
		}
	} else {
		log.Panicf("SDWA for VOP2 instruction opcode  %d not implemented \n", inst.Opcode)
	}

}

func (u *ALUImpl) runVADDI32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	inst := state.Inst()
	sp.VCC = 0

	var i uint
	if inst.IsSdwa == false {
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}

			src0 := asInt32(uint32(sp.SRC0[i]))
			src1 := asInt32(uint32(sp.SRC1[i]))

			if (src1 > 0 && src0 > math.MaxInt32-src1) ||
				(src1 < 0 && src0 < math.MinInt32+src1) {
				sp.VCC |= 1 << uint32(i)
			}

			sp.DST[i] = uint64(int32ToBits(src0 + src1))
		}
	} else {
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}
			src0 := asInt32(uint32(sp.SRC0[i]) & inst.Src0_Sel)
			src1 := asInt32(uint32(sp.SRC1[i]) & inst.Src1_Sel)
			if (src1 > 0 && src0 > math.MaxInt32-src1) ||
				(src1 < 0 && src0 < math.MinInt32+src1) {
				sp.VCC |= 1 << uint32(i)
			}
			result := int32ToBits((src0 + src1) & asInt32(inst.Dst_Sel))
			sp.DST[i] = uint64(result)
		}
	}
}

func (u *ALUImpl) runVSUBI32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	sp.VCC = 0
	inst := state.Inst()
	var i uint
	if inst.IsSdwa == false {
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}

			src0 := uint32(sp.SRC0[i])
			src1 := uint32(sp.SRC1[i])

			if src0 < src1 {
				sp.VCC |= 1 << uint32(i)
			}

			sp.DST[i] = uint64(src0 - src1)
		}
	} else {
		log.Panicf("SDWA for VOP2 instruction opcode  %d not implemented \n", inst.Opcode)

	}
}

func (u *ALUImpl) runVSUBREVI32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	sp.VCC = 0
	inst := state.Inst()
	var i uint
	if inst.IsSdwa == false {
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}

			src0 := uint32(sp.SRC0[i])
			src1 := uint32(sp.SRC1[i])

			if src0 > src1 {
				sp.VCC |= 1 << uint32(i)
			}

			sp.DST[i] = uint64(src1 - src0)
		}
	} else {
		log.Panicf("SDWA for VOP2 instruction opcode  %d not implemented \n", inst.Opcode)

	}
}

func (u *ALUImpl) runVADDCU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	newVCC := uint64(0)
	inst := state.Inst()
	var i uint

	if inst.IsSdwa == false {
		for i = 0; i < 64; i++ {
			if !u.laneMasked(sp.EXEC, i) {
				continue
			}

			carry := (sp.VCC & (1 << uint(i))) >> uint(i)

			if sp.SRC0[i] > math.MaxUint32-carry-sp.SRC1[i] {
				newVCC |= 1 << uint32(i)
			}

			sp.DST[i] = sp.SRC0[i] + sp.SRC1[i] + carry
		}
		sp.VCC = newVCC
	} else {
		log.Panicf("SDWA for VOP2 instruction opcode  %d not implemented \n", inst.Opcode)

	}
}
