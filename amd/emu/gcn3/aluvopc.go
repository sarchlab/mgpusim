package gcn3

import (
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/emu"
)

//nolint:gocyclo,funlen
func (u *ALU) runVOPC(state emu.InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0x41: // v_cmp_lt_f32
		u.runVCmpLtF32(state)
	case 0x42: // v_cmp_eq_f32
		u.runVCmpEqF32(state)
	case 0x43: // v_cmp_le_f32
		u.runVCmpLeF32(state)
	case 0x44: // v_cmp_gt_f32
		u.runVCmpGtF32(state)
	case 0x45: // v_cmp_lg_f32
		u.runVCmpLgF32(state)
	case 0x46: // v_cmp_lg_f32
		u.runVCmpGeF32(state)
	case 0x49: // v_cmp_nge_f32
		u.runVCmpNgeF32(state)
	case 0x4A: // v_cmp_nlg_f32
		u.runVCmpNlgF32(state)
	case 0x4B: // v_cmp_ngt_f32
		u.runVCmpNgtF32(state)
	case 0x4C: // v_cmp_nle_f32
		u.runVCmpNleF32(state)
	case 0x4D: // v_cmp_neq_f32
		u.runVCmpNeqF32(state)
	case 0x4E: // v_cmp_nlt_f32
		u.runVCmpNltF32(state)
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

func (u *ALU) runVCmpLtF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := math.Float32frombits(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 < src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpEqF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := math.Float32frombits(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 == src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpLeF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := math.Float32frombits(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 <= src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpLgF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := math.Float32frombits(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 != src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpGtF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := math.Float32frombits(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 > src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpGeF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := math.Float32frombits(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 >= src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpNgeF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := math.Float32frombits(uint32(state.ReadOperand(inst.Src1, i)))
		if !(src0 >= src1) {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpNlgF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := math.Float32frombits(uint32(state.ReadOperand(inst.Src1, i)))
		if !(src0 != src1) {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpNgtF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := math.Float32frombits(uint32(state.ReadOperand(inst.Src1, i)))
		if !(src0 > src1) {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpNleF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := math.Float32frombits(uint32(state.ReadOperand(inst.Src1, i)))
		if !(src0 <= src1) {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpNeqF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := math.Float32frombits(uint32(state.ReadOperand(inst.Src1, i)))
		if !(src0 == src1) {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpNltF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := math.Float32frombits(uint32(state.ReadOperand(inst.Src1, i)))
		if !(src0 < src1) {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpLtI32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		src0 := asInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := asInt32(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 < src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpLeI32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		src0 := asInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := asInt32(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 <= src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpGtI32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		src0 := asInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := asInt32(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 > src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpLgI32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		src0 := asInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := asInt32(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 != src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpGeI32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		src0 := asInt32(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := asInt32(uint32(state.ReadOperand(inst.Src1, i)))
		if src0 >= src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpLtU32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 < src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpEqU32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		if uint32(state.ReadOperand(inst.Src0, i)) == uint32(state.ReadOperand(inst.Src1, i)) {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpLeU32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 <= src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpGtU32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 > src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpNeU32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 != src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpGeU32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 >= src1 {
			vcc |= 1 << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpFU64(state emu.InstEmuState) {
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		// V_CMP_F always sets false
		vcc = vcc & ^(uint64(1) << uint(i))
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpLtU64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 < src1 {
			vcc |= uint64(1) << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpEqU64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 == src1 {
			vcc |= uint64(1) << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpLeU64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 <= src1 {
			vcc |= uint64(1) << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpGtU64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 > src1 {
			vcc |= uint64(1) << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpLgU64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 != src1 {
			vcc |= uint64(1) << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpGeU64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		src1 := state.ReadOperand(inst.Src1, i)
		if src0 >= src1 {
			vcc |= uint64(1) << uint(i)
		}
	}
	state.SetVCC(vcc)
}

func (u *ALU) runVCmpTruU64(state emu.InstEmuState) {
	exec := state.EXEC()
	var vcc uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		vcc |= uint64(1) << uint(i)
	}
	state.SetVCC(vcc)
}
