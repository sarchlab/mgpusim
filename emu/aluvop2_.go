package emu

import (
	"log"
	"math"
)

func (u *ALU) runVOP2(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runVCNDMASKB32(state)
	case 5:
		u.runVMULF32(state)
	case 22:
		u.runVMACF32(state)
	case 25:
		u.runVADDI32(state)
	case 26:
		u.runVSUBI32(state)
	case 28:
		u.runVADDCU32(state)
	default:
		log.Panicf("Opcode %d for VOP2 format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runVCNDMASKB32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()

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
}

func (u *ALU) runVMULF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()

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
}

func (u *ALU) runVMACF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()

	var dst float32
	var src0 float32
	var src1 float32

	var i uint
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
}

func (u *ALU) runVADDI32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()

	var i uint
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
}

func (u *ALU) runVSUBI32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()

	var i uint
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
}

func (u *ALU) runVADDCU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		carry := (sp.VCC & (1 << uint(i))) >> uint(i)

		if sp.SRC0[i] > math.MaxUint32-carry-sp.SRC1[i] {
			sp.VCC |= 1 << uint32(i)
		}

		sp.DST[i] = sp.SRC0[i] + sp.SRC1[i] + carry
	}
}
