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
	case 0x44: // v_cmp_gt_f32
		u.runVCmpGtF32(state)
	case 0xC1: // v_cmp_lt_i32
		u.runVCmpLtI32(state)
	case 0xC3: // v_cmp_le_i32
		u.runVCmpLeI32(state)
	case 0xC4: // v_cmp_gt_i32
		u.runVCmpGtI32(state)
	case 0xC5: // v_cmp_lg_i32
		u.runVCmpLgI32(state)
	case 0xC6: // v_cmp_ge_i32
		u.runVCmpGeI32(state)
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
	case 0xE8:
		u.runVCmpFU64(state)
	case 0xE9:
		u.runVCmpLtU64(state)
	case 0xEA:
		u.runVCmpEqU64(state)
	case 0xEB:
		u.runVCmpLeU64(state)
	case 0xEC:
		u.runVCmpGtU64(state)
	case 0xED:
		u.runVCmpLgU64(state)
	case 0xEE:
		u.runVCmpGeU64(state)
	case 0xEF:
		u.runVCmpTruU64(state)
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
		if !laneMasked(sp.EXEC, i) {
			continue
		}
		src0 = math.Float32frombits(uint32(sp.SRC0[i]))
		src1 = math.Float32frombits(uint32(sp.SRC1[i]))
		if src0 < src1 {
			sp.VCC = sp.VCC | (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpGtF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	var src0, src1 float32
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}
		src0 = math.Float32frombits(uint32(sp.SRC0[i]))
		src1 = math.Float32frombits(uint32(sp.SRC1[i]))
		if src0 > src1 {
			sp.VCC = sp.VCC | (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpLtI32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := asInt32(uint32(sp.SRC0[i]))
		src1 := asInt32(uint32(sp.SRC1[i]))
		if src0 < src1 {
			sp.VCC = sp.VCC | (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpLeI32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := asInt32(uint32(sp.SRC0[i]))
		src1 := asInt32(uint32(sp.SRC1[i]))
		if src0 <= src1 {
			sp.VCC = sp.VCC | (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpGtI32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := asInt32(uint32(sp.SRC0[i]))
		src1 := asInt32(uint32(sp.SRC1[i]))
		if src0 > src1 {
			sp.VCC = sp.VCC | (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpLgI32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := asInt32(uint32(sp.SRC0[i]))
		src1 := asInt32(uint32(sp.SRC1[i]))
		if src0 != src1 {
			sp.VCC = sp.VCC | (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpGeI32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := asInt32(uint32(sp.SRC0[i]))
		src1 := asInt32(uint32(sp.SRC1[i]))
		if src0 >= src1 {
			sp.VCC = sp.VCC | (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpLtU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
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
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		if uint32(sp.SRC0[i]) == uint32(sp.SRC1[i]) {
			sp.VCC = sp.VCC | (1 << i)
		}
	}

}

func (u *ALUImpl) runVCmpLeU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if laneMasked(sp.EXEC, i) {
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
		if laneMasked(sp.EXEC, i) {
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
		if laneMasked(sp.EXEC, i) {
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
		if laneMasked(sp.EXEC, i) {
			if sp.SRC0[i] >= sp.SRC1[i] {
				sp.VCC = sp.VCC | (1 << i)
			}
		}
	}
}

func (u *ALUImpl) runVCmpFU64(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if laneMasked(sp.EXEC, i) {
			sp.VCC = sp.VCC & ^(uint64(1) << i)
		}
	}
}

func (u *ALUImpl) runVCmpLtU64(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if laneMasked(sp.EXEC, i) {
			if sp.SRC0[i] < sp.SRC1[i] {
				sp.VCC = sp.VCC | (uint64(1) << i)
			}
		}
	}
}

func (u *ALUImpl) runVCmpEqU64(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if laneMasked(sp.EXEC, i) {
			if sp.SRC0[i] == sp.SRC1[i] {
				sp.VCC = sp.VCC | (uint64(1) << i)
			}
		}
	}
}

func (u *ALUImpl) runVCmpLeU64(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if laneMasked(sp.EXEC, i) {
			if sp.SRC0[i] <= sp.SRC1[i] {
				sp.VCC = sp.VCC | (uint64(1) << i)
			}
		}
	}
}

func (u *ALUImpl) runVCmpGtU64(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if laneMasked(sp.EXEC, i) {
			if sp.SRC0[i] > sp.SRC1[i] {
				sp.VCC = sp.VCC | (uint64(1) << i)
			}
		}
	}
}

func (u *ALUImpl) runVCmpLgU64(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if laneMasked(sp.EXEC, i) {
			if sp.SRC0[i] != sp.SRC1[i] {
				sp.VCC = sp.VCC | (uint64(1) << i)
			}
		}
	}
}

func (u *ALUImpl) runVCmpGeU64(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if laneMasked(sp.EXEC, i) {
			if sp.SRC0[i] >= sp.SRC1[i] {
				sp.VCC = sp.VCC | (uint64(1) << i)
			}
		}
	}
}

func (u *ALUImpl) runVCmpTruU64(state InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	sp.VCC = 0
	var i uint
	for i = 0; i < 64; i++ {
		if laneMasked(sp.EXEC, i) {
			sp.VCC = sp.VCC | (uint64(1) << i)
		}
	}
}
