package emu

import (
	"log"
	"math"
)

func (u *ALUImpl) runVOPC(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0x41: // v_cmp_lt_f32
		u.runVCmpLtF32(state)
	case 0xC9: // v_cmp_lt_u32
		u.runVCmpLtU32(state)
	case 0xCA: // v_cmp_eq_u32
		u.runVCmpEqU32(state)
	case 0xCB: // v_cmp_le_u32
		u.runVCmpLeU32(state)
	case 0xCC: // v_cmp_gt_u32
		u.runVCmpGtU32(state)
	case 0xCD: // v_cmp_ne_u32
		u.runVCmpNeU32(state)
	case 0xCE: // v_cmp_ge_u32
		u.runVCmpGeU32(state)
	default:
		log.Panicf("Opcode 0x%02X for VOPC format is not implemented", inst.Opcode)
	}
}

func (u *ALUImpl) runVCmpLtF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	var src0, src1 float32
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}
		src0 = math.Float32frombits(uint32(sp.SRC0[i]))
		src1 = math.Float32frombits(uint32(sp.SRC1[i]))
		if src0 < src1 {
			sp.VCC = sp.VCC | (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpLtU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		if sp.SRC0[i] < sp.SRC1[i] {
			sp.VCC = sp.VCC | (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpEqU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		if sp.SRC0[i] == sp.SRC1[i] {
			sp.VCC = sp.VCC | (1 << i)
		}
	}

}

func (u *ALUImpl) runVCmpLeU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if u.laneMasked(sp.EXEC, i) {
			if sp.SRC0[i] <= sp.SRC1[i] {
				sp.VCC = sp.VCC | (1 << i)
			}
		}
	}
}

func (u *ALUImpl) runVCmpGtU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if u.laneMasked(sp.EXEC, i) {
			if sp.SRC0[i] > sp.SRC1[i] {
				sp.VCC = sp.VCC | (1 << i)
			}
		}
	}
}

func (u *ALUImpl) runVCmpNeU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if u.laneMasked(sp.EXEC, i) {
			if sp.SRC0[i] != sp.SRC1[i] {
				sp.VCC = sp.VCC | (1 << i)
			}
		}
	}
}

func (u *ALUImpl) runVCmpGeU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if u.laneMasked(sp.EXEC, i) {
			if sp.SRC0[i] >= sp.SRC1[i] {
				sp.VCC = sp.VCC | (1 << i)
			}
		}
	}
}
