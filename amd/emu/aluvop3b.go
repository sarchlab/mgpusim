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
	case 282:
		u.runVSUBU32VOP3b(state)
	case 283:
		u.runVSUBREVU32VOP3b(state)
	case 284:
		u.runVADDCU32VOP3b(state)
	case 285:
		u.runVSUBBU32VOP3b(state)
	case 286:
		u.runVSUBBREVU32VOP3b(state)
	case 481:
		u.runVDIVSCALEF64(state)
	default:
		log.Panicf("Opcode %d for VOP3b format is not implemented", inst.Opcode)
	}

	u.vop3aPostprocess(state)
}

func (u *ALUImpl) runVADDU32VOP3b(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var sdst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		sum := src0 + src1
		state.WriteOperand(inst.Dst, i, sum&0xffffffff)
		if sum > 0xffffffff {
			sdst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.SDst, 0, sdst)
}

func (u *ALUImpl) runVSUBU32VOP3b(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var sdst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		diff := src0 - src1
		state.WriteOperand(inst.Dst, i, diff&0xffffffff)
		if src0 < src1 {
			sdst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.SDst, 0, sdst)
}

func (u *ALUImpl) runVSUBREVU32VOP3b(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var sdst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		diff := src1 - src0
		state.WriteOperand(inst.Dst, i, diff&0xffffffff)
		if diff > 0xffffffff {
			sdst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.SDst, 0, sdst)
}

func (u *ALUImpl) runVADDCU32VOP3b(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var sdst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		src2 := state.ReadOperand(inst.Src2, i)
		sum := src0 + src1 + ((src2 & (1 << uint(i))) >> uint(i))
		carry := uint64(0)
		if sum > 0xffffffff {
			carry = 1
		}
		sdst |= carry << uint(i)
		state.WriteOperand(inst.Dst, i, sum&0xffffffff)
	}
	state.WriteOperand(inst.SDst, 0, sdst)
}

func (u *ALUImpl) runVSUBBU32VOP3b(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var sdst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		src2 := state.ReadOperand(inst.Src2, i)
		diff := src0 - src1 - ((src2 & (1 << uint(i))) >> uint(i))
		carry := uint64(0)
		if diff > 0xffffffff {
			carry = 1
		}
		sdst |= carry << uint(i)
		state.WriteOperand(inst.Dst, i, diff&0xffffffff)
	}
	state.WriteOperand(inst.SDst, 0, sdst)
}

func (u *ALUImpl) runVSUBBREVU32VOP3b(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var sdst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		src2 := state.ReadOperand(inst.Src2, i)
		diff := src1 - src0 - ((src2 & (1 << uint(i))) >> uint(i))
		carry := uint64(0)
		if diff > 0xffffffff {
			carry = 1
		}
		sdst |= carry << uint(i)
		state.WriteOperand(inst.Dst, i, diff&0xffffffff)
	}
	state.WriteOperand(inst.SDst, 0, sdst)
}

//nolint:gocyclo,funlen
func (u *ALUImpl) runVDIVSCALEF64(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		src0Bits := state.ReadOperand(inst.Src0, i)
		src1Bits := state.ReadOperand(inst.Src1, i)
		src2Bits := state.ReadOperand(inst.Src2, i)

		// set to 0
		var vcc uint64

		src0 := math.Float64frombits(src0Bits)
		src1 := math.Float64frombits(src1Bits)
		src2 := math.Float64frombits(src2Bits)

		exponentSrc1 := (src1Bits << 1) >> 53
		exponentSrc2 := (src2Bits << 1) >> 53

		diffExpSrc2Src1 := int64(exponentSrc2) - int64(exponentSrc1)

		fractionSrc1 := (src1Bits << 12) >> 12

		reversedSrc1 := float64(1) / src1
		src2DivSrc1 := src2 / src1

		exponentRevSrc1 := (uint64(reversedSrc1) << 1) >> 53
		fractionRevSrc1 := (uint64(reversedSrc1) << 12) >> 12

		exponentSrc2DivSrc1 := (uint64(src2DivSrc1) << 1) >> 53
		fractionSrc2DivSrc1 := (uint64(src2DivSrc1) << 12) >> 12

		var dstVal uint64

		if src2 == 0 || src1 == 0 {
			dstVal = 0x7FFFFFFFFFFFFFFF // NaN
		} else if diffExpSrc2Src1 >= 768 {
			// N/D near MAX_FLOAT
			vcc = 1
			if src0 == src1 {
				dstVal = math.Float64bits(src0 * math.Pow(2.0, 128))
			}
		} else if exponentSrc1 == 0 && fractionSrc1 != 0 {
			// subnormal .. => DENORM
			dstVal = math.Float64bits(src0 * math.Pow(2.0, 128))
		} else if (exponentRevSrc1 == 0 && fractionRevSrc1 != 0) && (exponentSrc2DivSrc1 == 0 && fractionSrc2DivSrc1 != 0) {
			vcc = 1
			if src0 == src1 {
				dstVal = math.Float64bits(src0 * math.Pow(2.0, 128))
			}
		} else if exponentRevSrc1 == 0 && fractionRevSrc1 != 0 {
			dstVal = math.Float64bits(src0 * math.Pow(2.0, 128))
		} else if exponentSrc2DivSrc1 == 0 && fractionSrc2DivSrc1 != 0 {
			vcc = 1
			if src0 == src2 {
				dstVal = math.Float64bits(src0 * math.Pow(2.0, 128))
			}
		} else if exponentSrc2 <= 53 {
			// Numerator is tiny
			dstVal = math.Float64bits(src0 * math.Pow(2.0, 128))
		}

		state.WriteOperand(inst.Dst, i, dstVal)
		state.WriteOperand(inst.SDst, 0, vcc)
	}
}
