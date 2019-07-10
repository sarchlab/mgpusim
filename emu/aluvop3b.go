package emu

import (
	"log"
	"math"
)

func (u *ALUImpl) runVOP3B(state InstEmuState) {
	inst := state.Inst()

	u.vop3aPreprocess(state)

	switch inst.Opcode {
	case 281:
		u.runVADDU32VOP3b(state)
	case 284:
		u.runVADDCU32VOP3b(state)
	case 481:
		u.runVDIVSCALEF64(state)
	default:
		log.Panicf("Opcode %d for VOP3b format is not implemented", inst.Opcode)
	}

	u.vop3aPostprocess(state)
}

func (u *ALUImpl) runVADDU32VOP3b(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3B()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = sp.SRC1[i] + sp.SRC0[i]
		if sp.DST[i] > 0x100000000 {
			sp.VCC |= 1 << i
		}
	}
}

func (u *ALUImpl) runVADDCU32VOP3b(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3B()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = sp.SRC0[i] + sp.SRC1[i] + ((sp.SRC2[i] & (1 << i)) >> i)
		carry := uint64(0)
		if sp.DST[i] > 0xffffffff {
			carry = 1
		}
		sp.SDST |= carry << i
		sp.DST[i] &= 0xffffffff
	}
}

func (u *ALUImpl) runVDIVSCALEF64(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3B()
	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		// set to 0
		//sp.VCC = sp.VCC & ((1 << i) - 1)
		sp.VCC = 0

		src0 := math.Float64frombits(sp.SRC0[i])
		src1 := math.Float64frombits(sp.SRC1[i])
		src2 := math.Float64frombits(sp.SRC2[i])

		exponent_src1 := uint64((uint64(sp.SRC1[i]) << 1) >> 53)
		exponent_src2 := uint64((uint64(sp.SRC2[i]) << 1) >> 53)

		diff_exp_src2_src1 := int64(exponent_src2) - int64(exponent_src1)

		fraction_src1 := uint64((uint64(sp.SRC1[i]) << 12) >> 12)

		reversed_src1 := float64(1) / src1
		src2_div_src1 := src2 / src1

		exponent_rev_src1 := uint64((uint64(reversed_src1) << 1) >> 53)
		fraction_rev_src1 := uint64((uint64(reversed_src1) << 12) >> 12)

		exponent_src2_div_src1 := uint64((uint64(src2_div_src1) << 1) >> 53)
		fraction_src2_div_src1 := uint64((uint64(src2_div_src1) << 12) >> 12)

		if src2 == 0 || src1 == 0 {
			sp.DST[i] = 0x7FFFFFFFFFFFFFFF // NaN
		} else if diff_exp_src2_src1 >= 768 {
			// N/D near MAX_FLOAT
			//sp.VCC = sp.VCC | (1 << i)
			sp.VCC = 1
			if src0 == src1 {
				// Only scale the denominator
				sp.DST[i] = math.Float64bits(src0 * math.Pow(2.0, 128))
			}
		} else if exponent_src1 == 0 && fraction_src1 != 0 {
			// subnormal .. => DENORM
			sp.DST[i] = math.Float64bits(src0 * math.Pow(2.0, 128))
		} else if (exponent_rev_src1 == 0 && fraction_rev_src1 != 0) && (exponent_src2_div_src1 == 0 && fraction_src2_div_src1 != 0) {
			//sp.VCC = sp.VCC | (1 << i)
			sp.VCC = 1
			if src0 == src1 {
				// Only scale the denominator
				sp.DST[i] = math.Float64bits(src0 * math.Pow(2.0, 128))
			}
		} else if exponent_rev_src1 == 0 && fraction_rev_src1 != 0 {
			sp.DST[i] = math.Float64bits(src0 * math.Pow(2.0, 128))
		} else if exponent_src2_div_src1 == 0 && fraction_src2_div_src1 != 0 {
			//sp.VCC = sp.VCC | (1 << i)
			sp.VCC = 1
			if src0 == src2 {
				// Only scale the denominator
				sp.DST[i] = math.Float64bits(src0 * math.Pow(2.0, 128))
			}
		} else if exponent_src2 <= 53 {
			// Numerator is tiny
			sp.DST[i] = math.Float64bits(src0 * math.Pow(2.0, 128))
		}
	}
}
