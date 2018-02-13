package emu

import (
	"log"
)

func (u *ALU) runSOPK(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSMOVKI32(state)
	default:
		log.Panicf("Opcode %d for SOPK format is not implemented", inst.Opcode)
	}

}

func (u *ALU) runSMOVKI32(state InstEmuState) {
	sp := state.Scratchpad().AsSOPK()
	imm := asInt16(uint16(sp.IMM & 0xffff))
	sp.DST = uint64(imm);
	}
