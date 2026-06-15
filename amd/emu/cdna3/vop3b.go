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
	case 480:
		u.runVDIVSCALEF32(state)
	case 494:
		u.runVDIVSCALEF64(state)
	default:
		log.Panicf("Opcode %d for VOP3b format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runVADDU32VOP3b(state emu.InstEmuState) {
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

func (u *ALU) runVSUBU32VOP3b(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var sdst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		result := src0 - src1
		state.WriteOperand(inst.Dst, i, uint64(result))
		if src1 > src0 {
			sdst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.SDst, 0, sdst)
}

func (u *ALU) runVSUBREVU32VOP3b(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var sdst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := uint32(state.ReadOperand(inst.Src0, i))
		src1 := uint32(state.ReadOperand(inst.Src1, i))
		result := src1 - src0
		state.WriteOperand(inst.Dst, i, uint64(result))
		if src0 > src1 {
			sdst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.SDst, 0, sdst)
}

func (u *ALU) runVADDCU32VOP3b(state emu.InstEmuState) {
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
		carry := (src2 >> uint(i)) & 1
		sum := src0 + src1 + carry
		state.WriteOperand(inst.Dst, i, sum&0xffffffff)
		if sum > 0xffffffff {
			sdst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.SDst, 0, sdst)
}

func (u *ALU) runVSUBBU32VOP3b(state emu.InstEmuState) {
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
		borrow := (src2 >> uint(i)) & 1
		diff := src0 - src1 - borrow
		state.WriteOperand(inst.Dst, i, diff&0xffffffff)
		if src1+borrow > src0 {
			sdst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.SDst, 0, sdst)
}

func (u *ALU) runVSUBBREVU32VOP3b(state emu.InstEmuState) {
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
		borrow := (src2 >> uint(i)) & 1
		diff := src1 - src0 - borrow
		state.WriteOperand(inst.Dst, i, diff&0xffffffff)
		if src0+borrow > src1 {
			sdst |= 1 << uint(i)
		}
	}
	state.WriteOperand(inst.SDst, 0, sdst)
}

func (u *ALU) runVDIVSCALEF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var sdst uint64
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		src1 := math.Float32frombits(uint32(state.ReadOperand(inst.Src1, i)))
		src2 := math.Float32frombits(uint32(state.ReadOperand(inst.Src2, i)))

		// v_div_scale_f32: Part of software division sequence
		// Simplified: Returns src0, sets VCC bit if quotient is denormal
		dst := src0
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))

		// Set SDST bit if result might be denormal (simplified check)
		if src1 != 0 && src2 != 0 {
			quotient := src0 / src2
			bits := math.Float32bits(quotient)
			exp := (bits >> 23) & 0xFF
			if exp == 0 {
				sdst |= 1 << uint(i)
			}
		}
	}
	state.WriteOperand(inst.SDst, 0, sdst)
}

func (u *ALU) runVDIVSCALEF64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := math.Float64frombits(state.ReadOperand(inst.Src0, i))
		src1 := math.Float64frombits(state.ReadOperand(inst.Src1, i))
		src2 := math.Float64frombits(state.ReadOperand(inst.Src2, i))

		// Simplified implementation
		dst := src0
		if src1 != 0 && src2 != 0 {
			dst = src0
		}
		state.WriteOperand(inst.Dst, i, math.Float64bits(dst))
	}
}
