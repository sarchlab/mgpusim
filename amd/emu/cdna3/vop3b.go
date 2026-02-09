package cdna3

import (
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
)

func (u *ALU) runVOP3B(state emu.InstEmuState) {
	inst := state.Inst()
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
	case 494:
		u.runVDIVSCALEF64(state)
	default:
		log.Panicf("Opcode %d for VOP3b format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runVADDU32VOP3b(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3B()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]
		result := src0 + src1
		sp.DST[i] = result & 0xFFFFFFFF
		if result > 0xFFFFFFFF {
			sp.SDST |= (1 << i)
		}
	}
}

func (u *ALU) runVSUBU32VOP3b(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3B()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := uint32(sp.SRC0[i])
		src1 := uint32(sp.SRC1[i])
		result := src0 - src1
		sp.DST[i] = uint64(result)
		if src1 > src0 {
			sp.SDST |= (1 << i)
		}
	}
}

func (u *ALU) runVSUBREVU32VOP3b(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3B()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := uint32(sp.SRC0[i])
		src1 := uint32(sp.SRC1[i])
		result := src1 - src0
		sp.DST[i] = uint64(result)
		if src0 > src1 {
			sp.SDST |= (1 << i)
		}
	}
}

func (u *ALU) runVADDCU32VOP3b(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3B()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]
		carry := (sp.SRC2[i] >> i) & 1
		result := src0 + src1 + carry
		sp.DST[i] = result & 0xFFFFFFFF
		if result > 0xFFFFFFFF {
			sp.SDST |= (1 << i)
		}
	}
}

func (u *ALU) runVSUBBU32VOP3b(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3B()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]
		borrow := (sp.SRC2[i] >> i) & 1
		result := src0 - src1 - borrow
		sp.DST[i] = result & 0xFFFFFFFF
		if src1+borrow > src0 {
			sp.SDST |= (1 << i)
		}
	}
}

func (u *ALU) runVSUBBREVU32VOP3b(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3B()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]
		borrow := (sp.SRC2[i] >> i) & 1
		result := src1 - src0 - borrow
		sp.DST[i] = result & 0xFFFFFFFF
		if src0+borrow > src1 {
			sp.SDST |= (1 << i)
		}
	}
}

func (u *ALU) runVDIVSCALEF64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP3B()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float64frombits(sp.SRC0[i])
		src1 := math.Float64frombits(sp.SRC1[i])
		src2 := math.Float64frombits(sp.SRC2[i])

		// Simplified implementation
		dst := src0
		if src1 != 0 && src2 != 0 {
			dst = src0
		}
		sp.DST[i] = math.Float64bits(dst)
	}
}
