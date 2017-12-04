package emu

import "log"

func (u *ALU) runVOPC(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
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

func (u *ALU) runVCmpLtU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
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

func (u *ALU) runVCmpEqU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
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

func (u *ALU) runVCmpLeU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if u.laneMasked(sp.EXEC, i) {
			if sp.SRC0[i] <= sp.SRC1[i] {
				sp.VCC = sp.VCC | (1 << i)
			}
		}
	}
}

func (u *ALU) runVCmpGtU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if u.laneMasked(sp.EXEC, i) {
			if sp.SRC0[i] > sp.SRC1[i] {
				sp.VCC = sp.VCC | (1 << i)
			}
		}
	}
}

func (u *ALU) runVCmpNeU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if u.laneMasked(sp.EXEC, i) {
			if sp.SRC0[i] != sp.SRC1[i] {
				sp.VCC = sp.VCC | (1 << i)
			}
		}
	}
}

func (u *ALU) runVCmpGeU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if u.laneMasked(sp.EXEC, i) {
			if sp.SRC0[i] >= sp.SRC1[i] {
				sp.VCC = sp.VCC | (1 << i)
			}
		}
	}
}
