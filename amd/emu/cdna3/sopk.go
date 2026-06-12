package cdna3

import (
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
)

func (u *ALU) runSOPK(state emu.InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSMOVKI32(state)
	case 1:
		u.runSCMOVKI32(state)
	case 2:
		u.runSCMPKEQI32(state)
	case 3:
		u.runSCMPKLGI32(state)
	case 14: // s_addk_i32
		u.runSADDKI32(state)
	case 15:
		u.runSMULKI32(state)
	default:
		log.Panicf("Opcode %d for SOPK format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runSMOVKI32(state emu.InstEmuState) {
	inst := state.Inst()
	imm := emu.AsInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff))
	state.WriteOperand(inst.Dst, 0, uint64(emu.Int16ToBits(imm)))
}

func (u *ALU) runSCMOVKI32(state emu.InstEmuState) {
	inst := state.Inst()
	if state.SCC() == 1 {
		imm := emu.AsInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff))
		state.WriteOperand(inst.Dst, 0, uint64(emu.Int16ToBits(imm)))
	}
}

func (u *ALU) runSCMPKEQI32(state emu.InstEmuState) {
	inst := state.Inst()
	imm := int32(emu.AsInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff)))
	src := emu.AsInt32(uint32(state.ReadOperand(inst.Dst, 0)))
	if src == imm {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSCMPKLGI32(state emu.InstEmuState) {
	inst := state.Inst()
	imm := int32(emu.AsInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff)))
	src := emu.AsInt32(uint32(state.ReadOperand(inst.Dst, 0)))
	if src != imm {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSMULKI32(state emu.InstEmuState) {
	inst := state.Inst()
	imm := int32(emu.AsInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff)))
	src := emu.AsInt32(uint32(state.ReadOperand(inst.Dst, 0)))
	result := src * imm
	state.WriteOperand(inst.Dst, 0, uint64(emu.Int32ToBits(result)))
}

// runSADDKI32 implements s_addk_i32 (opcode 14)
// D.i = D.i + signext(SIMM16); SCC = overflow
func (u *ALU) runSADDKI32(state emu.InstEmuState) {
	inst := state.Inst()
	imm := int32(emu.AsInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff)))
	src := emu.AsInt32(uint32(state.ReadOperand(inst.Dst, 0)))
	result := src + imm
	state.WriteOperand(inst.Dst, 0, uint64(emu.Int32ToBits(result)))

	// Set SCC on signed overflow
	if (src > 0 && imm > 0 && result < 0) || (src < 0 && imm < 0 && result > 0) {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}
