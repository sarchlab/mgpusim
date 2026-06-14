package gcn3

import (
	"log"

	"github.com/sarchlab/mgpusim/v5/amd/emu"
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
	case 15:
		u.runSMULKI32(state)
	default:
		log.Panicf("Opcode %d for SOPK format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runSMOVKI32(state emu.InstEmuState) {
	inst := state.Inst()
	imm := asInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff))
	state.WriteOperand(inst.Dst, 0, uint64(imm))
}

func (u *ALU) runSCMOVKI32(state emu.InstEmuState) {
	inst := state.Inst()
	if state.SCC() == 1 {
		imm := asInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff))
		state.WriteOperand(inst.Dst, 0, uint64(imm))
	}
}

func (u *ALU) runSCMPKEQI32(state emu.InstEmuState) {
	inst := state.Inst()
	imm := asInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff))
	dst := state.ReadOperand(inst.Dst, 0)
	if asInt16(uint16(dst)) == imm {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSCMPKLGI32(state emu.InstEmuState) {
	inst := state.Inst()
	imm := asInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff))
	dst := state.ReadOperand(inst.Dst, 0)
	if asInt16(uint16(dst)) != imm {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSMULKI32(state emu.InstEmuState) {
	inst := state.Inst()
	imm := asInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff))
	dst := asInt32(uint32(state.ReadOperand(inst.Dst, 0)))

	state.WriteOperand(inst.Dst, 0, int64ToBits(int64(int32(imm)*dst)))
}
